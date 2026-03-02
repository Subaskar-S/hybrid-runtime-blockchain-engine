use crate::state::StateEngine;
use crate::types::Block;
use lazy_static::lazy_static;
use std::sync::Mutex;

lazy_static! {
    static ref ENGINE: Mutex<Option<StateEngine>> = Mutex::new(None);
}

/// Initialize the state engine
/// Returns: 0 = success, -1 = error
#[no_mangle]
pub extern "C" fn init_engine() -> i32 {
    let mut engine_guard = match ENGINE.lock() {
        Ok(guard) => guard,
        Err(_) => return -1,
    };

    *engine_guard = Some(StateEngine::new());
    0
}

/// Apply a block to the state
/// data_ptr: pointer to serialized block data
/// data_len: length of serialized data
/// result_ptr: output pointer for state root (32 bytes)
/// result_len: output length (should be 32)
/// Returns: 0 = success, -1 = invalid input, -2 = serialization error, -3 = state error
#[no_mangle]
pub extern "C" fn apply_block(
    data_ptr: *const u8,
    data_len: usize,
    result_ptr: *mut *mut u8,
    result_len: *mut usize,
) -> i32 {
    // Validate input pointers
    if data_ptr.is_null() || result_ptr.is_null() || result_len.is_null() {
        eprintln!("FFI Error: Null pointer provided");
        return -1;
    }

    // Validate length
    if data_len == 0 || data_len > 10 * 1024 * 1024 {
        eprintln!("FFI Error: Invalid data length: {}", data_len);
        return -1;
    }

    // Convert to slice
    let data = unsafe { std::slice::from_raw_parts(data_ptr, data_len) };

    // Deserialize block
    let block: Block = match deserialize_block(data) {
        Ok(b) => b,
        Err(e) => {
            eprintln!("FFI Error: Deserialization failed: {}", e);
            return -2;
        }
    };

    // Get engine
    let mut engine_guard = match ENGINE.lock() {
        Ok(guard) => guard,
        Err(_) => {
            eprintln!("FFI Error: Failed to lock engine");
            return -3;
        }
    };

    let engine = match engine_guard.as_mut() {
        Some(e) => e,
        None => {
            eprintln!("FFI Error: Engine not initialized");
            return -3;
        }
    };

    // Apply block
    let state_root = match engine.apply_block(&block) {
        Ok(root) => root,
        Err(e) => {
            eprintln!("FFI Error: Apply block failed: {}", e);
            return -3;
        }
    };

    // Allocate result buffer
    let mut result = state_root.to_vec();
    unsafe {
        *result_len = result.len();
        *result_ptr = result.as_mut_ptr();
    }
    std::mem::forget(result); // Prevent deallocation

    0
}

/// Rollback state to a specific block number
/// block_number: target block number
/// Returns: 0 = success, -1 = error
#[no_mangle]
pub extern "C" fn rollback_to(block_number: u64) -> i32 {
    let mut engine_guard = match ENGINE.lock() {
        Ok(guard) => guard,
        Err(_) => return -1,
    };

    let engine = match engine_guard.as_mut() {
        Some(e) => e,
        None => return -1,
    };

    match engine.rollback_to(block_number) {
        Ok(_) => 0,
        Err(e) => {
            eprintln!("FFI Error: Rollback failed: {}", e);
            -1
        }
    }
}

/// Get current state root
/// root_ptr: output pointer for state root (32 bytes)
/// root_len: output length (should be 32)
/// Returns: 0 = success, -1 = error
#[no_mangle]
pub extern "C" fn get_state_root(root_ptr: *mut *mut u8, root_len: *mut usize) -> i32 {
    if root_ptr.is_null() || root_len.is_null() {
        return -1;
    }

    let engine_guard = match ENGINE.lock() {
        Ok(guard) => guard,
        Err(_) => return -1,
    };

    let engine = match engine_guard.as_ref() {
        Some(e) => e,
        None => return -1,
    };

    let state_root = engine.state_root();
    let mut result = state_root.to_vec();

    unsafe {
        *root_len = result.len();
        *root_ptr = result.as_mut_ptr();
    }
    std::mem::forget(result);

    0
}

/// Get statistics (memory usage, state size, etc.)
/// stats_ptr: output pointer for JSON stats
/// stats_len: output length
/// Returns: 0 = success, -1 = error
#[no_mangle]
pub extern "C" fn get_stats(stats_ptr: *mut *mut u8, stats_len: *mut usize) -> i32 {
    if stats_ptr.is_null() || stats_len.is_null() {
        return -1;
    }

    let engine_guard = match ENGINE.lock() {
        Ok(guard) => guard,
        Err(_) => return -1,
    };

    let engine = match engine_guard.as_ref() {
        Some(e) => e,
        None => return -1,
    };

    let stats = serde_json::json!({
        "block_number": engine.block_number(),
        "state_size": engine.state_size(),
        "history_length": engine.history_length(),
        "memory_usage_bytes": engine.memory_usage(),
    });

    let stats_json = match serde_json::to_vec(&stats) {
        Ok(json) => json,
        Err(_) => return -1,
    };

    let mut result = stats_json;
    unsafe {
        *stats_len = result.len();
        *stats_ptr = result.as_mut_ptr();
    }
    std::mem::forget(result);

    0
}

/// Free a buffer allocated by Rust
/// ptr: pointer to buffer
/// len: length of buffer
#[no_mangle]
pub extern "C" fn free_buffer(ptr: *mut u8, len: usize) {
    if !ptr.is_null() && len > 0 {
        unsafe {
            let _ = Vec::from_raw_parts(ptr, len, len);
            // Vec will be dropped and memory freed
        }
    }
}

/// Deserialize block from binary format
/// Format:
/// [Version: 1 byte]
/// [Block Number: 8 bytes, big-endian]
/// [Parent Hash: 32 bytes]
/// [Timestamp: 8 bytes, big-endian]
/// [Tx Count: 4 bytes, big-endian]
/// [Transactions: variable length]
fn deserialize_block(data: &[u8]) -> Result<Block, String> {
    if data.len() < 53 {
        // Minimum: 1 + 8 + 32 + 8 + 4
        return Err("Data too short".to_string());
    }

    let mut offset = 0;

    // Version byte
    let version = data[offset];
    offset += 1;
    if version != 1 {
        return Err(format!("Unsupported version: {}", version));
    }

    // Block number
    let block_number = u64::from_be_bytes(
        data[offset..offset + 8]
            .try_into()
            .map_err(|_| "Invalid block number")?,
    );
    offset += 8;

    // Parent hash
    let parent_hash: [u8; 32] = data[offset..offset + 32]
        .try_into()
        .map_err(|_| "Invalid parent hash")?;
    offset += 32;

    // Timestamp
    let timestamp = u64::from_be_bytes(
        data[offset..offset + 8]
            .try_into()
            .map_err(|_| "Invalid timestamp")?,
    );
    offset += 8;

    // Transaction count
    let tx_count = u32::from_be_bytes(
        data[offset..offset + 4]
            .try_into()
            .map_err(|_| "Invalid tx count")?,
    );
    offset += 4;

    // Transactions
    let mut transactions = Vec::new();
    for _ in 0..tx_count {
        if offset + 20 + 20 + 32 + 4 > data.len() {
            return Err("Truncated transaction data".to_string());
        }

        // From address
        let from: [u8; 20] = data[offset..offset + 20]
            .try_into()
            .map_err(|_| "Invalid from address")?;
        offset += 20;

        // To address
        let to: [u8; 20] = data[offset..offset + 20]
            .try_into()
            .map_err(|_| "Invalid to address")?;
        offset += 20;

        // Value (U256 as 32 bytes)
        let value_bytes: [u8; 32] = data[offset..offset + 32]
            .try_into()
            .map_err(|_| "Invalid value")?;
        offset += 32;

        // Data length
        let data_len = u32::from_be_bytes(
            data[offset..offset + 4]
                .try_into()
                .map_err(|_| "Invalid data length")?,
        ) as usize;
        offset += 4;

        // Data
        if offset + data_len > data.len() {
            return Err("Truncated transaction data".to_string());
        }
        let tx_data = data[offset..offset + data_len].to_vec();
        offset += data_len;

        transactions.push(crate::types::Transaction {
            from,
            to,
            value: crate::types::U256(value_bytes),
            data: tx_data,
        });
    }

    Ok(Block {
        number: block_number,
        parent_hash,
        timestamp,
        transactions,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_init_engine() {
        let result = init_engine();
        assert_eq!(result, 0);
    }

    #[test]
    fn test_deserialize_block_empty() {
        let mut data = Vec::new();
        data.push(1u8); // Version
        data.extend_from_slice(&1u64.to_be_bytes()); // Block number
        data.extend_from_slice(&[0u8; 32]); // Parent hash
        data.extend_from_slice(&1234567890u64.to_be_bytes()); // Timestamp
        data.extend_from_slice(&0u32.to_be_bytes()); // Tx count

        let block = deserialize_block(&data).unwrap();
        assert_eq!(block.number, 1);
        assert_eq!(block.transactions.len(), 0);
    }

    #[test]
    fn test_deserialize_block_invalid_version() {
        let mut data = Vec::new();
        data.push(99u8); // Invalid version
        data.extend_from_slice(&1u64.to_be_bytes());
        data.extend_from_slice(&[0u8; 32]);
        data.extend_from_slice(&1234567890u64.to_be_bytes());
        data.extend_from_slice(&0u32.to_be_bytes());

        let result = deserialize_block(&data);
        assert!(result.is_err());
    }

    #[test]
    fn test_free_buffer() {
        let data = vec![1u8, 2, 3, 4, 5];
        let ptr = data.as_ptr() as *mut u8;
        let len = data.len();
        std::mem::forget(data);

        // Should not crash
        free_buffer(ptr, len);
    }
}
