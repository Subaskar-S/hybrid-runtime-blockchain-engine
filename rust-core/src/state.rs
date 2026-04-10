#[allow(unused_imports)]
use crate::types::{Account, Address, Block, Transaction, U256};
use std::collections::HashMap;

/// State snapshot for rollback support
#[derive(Debug, Clone)]
pub struct StateSnapshot {
    pub block_number: u64,
    pub state: HashMap<Address, Account>,
    pub state_root: [u8; 32],
}

/// State engine - maintains blockchain state deterministically
#[derive(Debug)]
pub struct StateEngine {
    /// Current state map: address -> account
    state: HashMap<Address, Account>,
    /// Current block number
    block_number: u64,
    /// Current state root hash
    state_root: [u8; 32],
    /// History of state snapshots for rollback
    history: Vec<StateSnapshot>,
}

impl StateEngine {
    /// Create a new state engine
    pub fn new() -> Self {
        StateEngine {
            state: HashMap::new(),
            block_number: 0,
            state_root: [0u8; 32],
            history: Vec::new(),
        }
    }

    /// Get current block number
    pub fn block_number(&self) -> u64 {
        self.block_number
    }

    /// Get current state root
    pub fn state_root(&self) -> [u8; 32] {
        self.state_root
    }

    /// Get account state (returns default if not found)
    pub fn get_account(&self, address: &Address) -> Account {
        self.state.get(address).cloned().unwrap_or_default()
    }

    /// Set account state
    pub fn set_account(&mut self, address: Address, account: Account) {
        self.state.insert(address, account);
    }

    /// Get state size (number of accounts)
    pub fn state_size(&self) -> usize {
        self.state.len()
    }

    /// Get history length
    pub fn history_length(&self) -> usize {
        self.history.len()
    }

    /// Calculate state root using Blake3
    /// Deterministic hash of all account states
    fn calculate_state_root(&self) -> [u8; 32] {
        // Sort addresses for deterministic ordering
        let mut addresses: Vec<&Address> = self.state.keys().collect();
        addresses.sort();

        // Hash all accounts in order
        let mut hasher = blake3::Hasher::new();
        for addr in addresses {
            hasher.update(addr);
            if let Some(account) = self.state.get(addr) {
                hasher.update(&account.balance.0);
                hasher.update(&account.nonce.to_be_bytes());
            }
        }

        hasher.finalize().into()
    }

    /// Apply a block to the state
    pub fn apply_block(&mut self, block: &Block) -> Result<[u8; 32], String> {
        // Validate block number is sequential
        if block.number != self.block_number + 1 {
            return Err(format!(
                "Invalid block number: expected {}, got {}",
                self.block_number + 1,
                block.number
            ));
        }

        // Save snapshot before applying block
        let snapshot = StateSnapshot {
            block_number: self.block_number,
            state: self.state.clone(),
            state_root: self.state_root,
        };
        self.history.push(snapshot);

        // Process transactions
        for tx in &block.transactions {
            self.apply_transaction(tx)?;
        }

        // Update block number
        self.block_number = block.number;

        // Calculate new state root
        self.state_root = self.calculate_state_root();

        Ok(self.state_root)
    }

    /// Apply a single transaction
    fn apply_transaction(&mut self, tx: &Transaction) -> Result<(), String> {
        // Get sender account
        let mut sender = self.get_account(&tx.from);

        // Check sufficient balance
        if sender.balance.checked_sub(&tx.value).is_none() {
            return Err(format!("Insufficient balance for transaction"));
        }

        // Deduct from sender
        sender.balance = sender
            .balance
            .checked_sub(&tx.value)
            .ok_or("Balance underflow")?;
        sender.nonce += 1;
        self.set_account(tx.from, sender);

        // Add to receiver
        let mut receiver = self.get_account(&tx.to);
        receiver.balance = receiver
            .balance
            .checked_add(&tx.value)
            .ok_or("Balance overflow")?;
        self.set_account(tx.to, receiver);

        Ok(())
    }

    /// Rollback state to a specific block number
    pub fn rollback_to(&mut self, target_block: u64) -> Result<(), String> {
        // Find snapshot at target block
        let snapshot_idx = self
            .history
            .iter()
            .position(|s| s.block_number == target_block)
            .ok_or(format!("No snapshot found for block {}", target_block))?;

        // Restore state from snapshot
        let snapshot = &self.history[snapshot_idx];
        self.state = snapshot.state.clone();
        self.block_number = snapshot.block_number;
        self.state_root = snapshot.state_root;

        // Truncate history after rollback point
        self.history.truncate(snapshot_idx + 1);

        Ok(())
    }

    /// Get memory usage statistics
    pub fn memory_usage(&self) -> usize {
        // Approximate memory usage
        let state_size = self.state.len() * (20 + std::mem::size_of::<Account>());
        let history_size = self.history.len()
            * (std::mem::size_of::<StateSnapshot>()
                + self.state.len() * (20 + std::mem::size_of::<Account>()));
        state_size + history_size
    }
}

impl Default for StateEngine {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_block(number: u64, parent_hash: [u8; 32]) -> Block {
        Block {
            number,
            parent_hash,
            timestamp: 1234567890 + number,
            transactions: vec![],
        }
    }

    fn create_test_transaction(from: Address, to: Address, value: u64) -> Transaction {
        Transaction {
            from,
            to,
            value: U256::from_u64(value),
            data: vec![],
        }
    }

    #[test]
    fn test_state_engine_new() {
        let engine = StateEngine::new();
        assert_eq!(engine.block_number(), 0);
        assert_eq!(engine.state_size(), 0);
    }

    #[test]
    fn test_apply_block_empty() {
        let mut engine = StateEngine::new();
        let block = create_test_block(1, [0u8; 32]);
        let result = engine.apply_block(&block);
        assert!(result.is_ok());
        assert_eq!(engine.block_number(), 1);
    }

    #[test]
    fn test_apply_block_invalid_number() {
        let mut engine = StateEngine::new();
        let block = create_test_block(5, [0u8; 32]); // Skip blocks
        let result = engine.apply_block(&block);
        assert!(result.is_err());
    }

    #[test]
    fn test_apply_transaction() {
        let mut engine = StateEngine::new();

        // Setup: give sender initial balance
        let sender: Address = [1u8; 20];
        let receiver: Address = [2u8; 20];
        engine.set_account(sender, Account::with_balance(U256::from_u64(1000)));

        // Create block with transaction
        let tx = create_test_transaction(sender, receiver, 300);
        let block = Block {
            number: 1,
            parent_hash: [0u8; 32],
            timestamp: 1234567890,
            transactions: vec![tx],
        };

        let result = engine.apply_block(&block);
        assert!(result.is_ok());

        // Verify balances
        let sender_account = engine.get_account(&sender);
        let receiver_account = engine.get_account(&receiver);
        assert_eq!(sender_account.balance, U256::from_u64(700));
        assert_eq!(sender_account.nonce, 1);
        assert_eq!(receiver_account.balance, U256::from_u64(300));
    }

    #[test]
    fn test_apply_transaction_insufficient_balance() {
        let mut engine = StateEngine::new();

        let sender: Address = [1u8; 20];
        let receiver: Address = [2u8; 20];
        engine.set_account(sender, Account::with_balance(U256::from_u64(100)));

        let tx = create_test_transaction(sender, receiver, 200); // More than balance
        let block = Block {
            number: 1,
            parent_hash: [0u8; 32],
            timestamp: 1234567890,
            transactions: vec![tx],
        };

        let result = engine.apply_block(&block);
        assert!(result.is_err());
    }

    #[test]
    fn test_rollback() {
        let mut engine = StateEngine::new();

        // Apply block 1
        let block1 = create_test_block(1, [0u8; 32]);
        engine.apply_block(&block1).unwrap();

        // Apply block 2
        let block2 = create_test_block(2, block1.hash());
        engine.apply_block(&block2).unwrap();

        assert_eq!(engine.block_number(), 2);

        // Rollback to block 1
        engine.rollback_to(1).unwrap();
        assert_eq!(engine.block_number(), 1);
    }

    #[test]
    fn test_state_root_deterministic() {
        let mut engine1 = StateEngine::new();
        let mut engine2 = StateEngine::new();

        let sender: Address = [1u8; 20];
        let receiver: Address = [2u8; 20];

        // Apply same transactions to both engines
        engine1.set_account(sender, Account::with_balance(U256::from_u64(1000)));
        engine2.set_account(sender, Account::with_balance(U256::from_u64(1000)));

        let tx = create_test_transaction(sender, receiver, 300);
        let block = Block {
            number: 1,
            parent_hash: [0u8; 32],
            timestamp: 1234567890,
            transactions: vec![tx.clone()],
        };

        engine1.apply_block(&block).unwrap();
        engine2.apply_block(&block).unwrap();

        // State roots should be identical
        assert_eq!(engine1.state_root(), engine2.state_root());
    }
}
