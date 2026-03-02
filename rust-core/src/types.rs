use serde::{Deserialize, Serialize};
use std::fmt;

/// Address type alias for 20-byte Ethereum addresses
pub type Address = [u8; 20];

/// U256 type for large integers (balance values)
/// Using [u8; 32] for simplicity and determinism
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub struct U256(pub [u8; 32]);

impl U256 {
    pub fn zero() -> Self {
        U256([0u8; 32])
    }

    pub fn from_u64(value: u64) -> Self {
        let mut bytes = [0u8; 32];
        bytes[24..32].copy_from_slice(&value.to_be_bytes());
        U256(bytes)
    }

    /// Checked addition - returns None on overflow
    pub fn checked_add(&self, other: &U256) -> Option<U256> {
        let mut result = [0u8; 32];
        let mut carry = 0u16;

        // Add from least significant byte
        for i in (0..32).rev() {
            let sum = self.0[i] as u16 + other.0[i] as u16 + carry;
            result[i] = sum as u8;
            carry = sum >> 8;
        }

        if carry > 0 {
            None // Overflow
        } else {
            Some(U256(result))
        }
    }

    /// Checked subtraction - returns None on underflow
    pub fn checked_sub(&self, other: &U256) -> Option<U256> {
        let mut result = [0u8; 32];
        let mut borrow = 0i16;

        // Subtract from least significant byte
        for i in (0..32).rev() {
            let diff = self.0[i] as i16 - other.0[i] as i16 - borrow;
            if diff < 0 {
                result[i] = (diff + 256) as u8;
                borrow = 1;
            } else {
                result[i] = diff as u8;
                borrow = 0;
            }
        }

        if borrow > 0 {
            None // Underflow
        } else {
            Some(U256(result))
        }
    }
}

impl fmt::Display for U256 {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "0x")?;
        for byte in &self.0 {
            write!(f, "{:02x}", byte)?;
        }
        Ok(())
    }
}

/// Account state
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Account {
    pub balance: U256,
    pub nonce: u64,
}

impl Account {
    pub fn new() -> Self {
        Account {
            balance: U256::zero(),
            nonce: 0,
        }
    }

    pub fn with_balance(balance: U256) -> Self {
        Account { balance, nonce: 0 }
    }
}

impl Default for Account {
    fn default() -> Self {
        Self::new()
    }
}

/// Transaction data
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Transaction {
    pub from: Address,
    pub to: Address,
    pub value: U256,
    pub data: Vec<u8>,
}

/// Block data
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Block {
    pub number: u64,
    pub parent_hash: [u8; 32],
    pub timestamp: u64,
    pub transactions: Vec<Transaction>,
}

impl Block {
    /// Calculate block hash using Blake3
    pub fn hash(&self) -> [u8; 32] {
        let serialized = serde_json::to_vec(self).unwrap();
        blake3::hash(&serialized).into()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_u256_zero() {
        let zero = U256::zero();
        assert_eq!(zero.0, [0u8; 32]);
    }

    #[test]
    fn test_u256_from_u64() {
        let value = U256::from_u64(1000);
        let mut expected = [0u8; 32];
        expected[24..32].copy_from_slice(&1000u64.to_be_bytes());
        assert_eq!(value.0, expected);
    }

    #[test]
    fn test_u256_checked_add() {
        let a = U256::from_u64(100);
        let b = U256::from_u64(200);
        let result = a.checked_add(&b).unwrap();
        let expected = U256::from_u64(300);
        assert_eq!(result, expected);
    }

    #[test]
    fn test_u256_checked_sub() {
        let a = U256::from_u64(300);
        let b = U256::from_u64(100);
        let result = a.checked_sub(&b).unwrap();
        let expected = U256::from_u64(200);
        assert_eq!(result, expected);
    }

    #[test]
    fn test_u256_sub_underflow() {
        let a = U256::from_u64(100);
        let b = U256::from_u64(200);
        assert!(a.checked_sub(&b).is_none());
    }

    #[test]
    fn test_account_new() {
        let account = Account::new();
        assert_eq!(account.balance, U256::zero());
        assert_eq!(account.nonce, 0);
    }

    #[test]
    fn test_block_hash() {
        let block = Block {
            number: 1,
            parent_hash: [0u8; 32],
            timestamp: 1234567890,
            transactions: vec![],
        };
        let hash1 = block.hash();
        let hash2 = block.hash();
        assert_eq!(hash1, hash2); // Deterministic
    }
}
