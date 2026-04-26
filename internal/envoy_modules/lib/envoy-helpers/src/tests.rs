#![allow(clippy::unwrap_used, clippy::expect_used)]

use super::EnvoyBuffersReader;
use envoy_proxy_dynamic_modules_rust_sdk::EnvoyMutBuffer;
use std::io::Read;

#[test]
fn test_envoy_buffers_reader_empty_buffers() {
    let mut reader = EnvoyBuffersReader::new(vec![]);
    let mut out = Vec::new();
    reader.read_to_end(&mut out).unwrap();
    assert!(out.is_empty());
}

#[test]
#[allow(static_mut_refs)]
fn test_envoy_buffers_reader_single_chunk() {
    static mut CHUNK: [u8; 5] = *b"hello";
    let buffers = vec![EnvoyMutBuffer::new(unsafe { &mut CHUNK })];
    let mut reader = EnvoyBuffersReader::new(buffers);
    let mut out = Vec::new();
    reader.read_to_end(&mut out).unwrap();
    assert_eq!(out, b"hello");
}

#[test]
#[allow(static_mut_refs)]
fn test_envoy_buffers_reader_multiple_chunks() {
    static mut CHUNK_X: [u8; 3] = *b"foo";
    static mut CHUNK_Y: [u8; 1] = *b"-";
    static mut CHUNK_Z: [u8; 3] = *b"bar";
    let buffers = vec![
        EnvoyMutBuffer::new(unsafe { &mut CHUNK_X }),
        EnvoyMutBuffer::new(unsafe { &mut CHUNK_Y }),
        EnvoyMutBuffer::new(unsafe { &mut CHUNK_Z }),
    ];
    let mut reader = EnvoyBuffersReader::new(buffers);
    let mut out = Vec::new();
    reader.read_to_end(&mut out).unwrap();
    assert_eq!(out, b"foo-bar");
}

#[test]
#[allow(static_mut_refs)]
fn test_envoy_buffers_reader_small_read_buf() {
    // Read buffer smaller than a single chunk — verifies partial-read and
    // offset advancement within a chunk.
    static mut CHUNK_X: [u8; 6] = *b"abcdef";
    static mut CHUNK_Y: [u8; 5] = *b"ghijk";
    let buffers = vec![
        EnvoyMutBuffer::new(unsafe { &mut CHUNK_X }),
        EnvoyMutBuffer::new(unsafe { &mut CHUNK_Y }),
    ];
    let mut reader = EnvoyBuffersReader::new(buffers);

    let mut tmp = [0u8; 4];
    let n1 = reader.read(&mut tmp).unwrap();
    assert_eq!(n1, 4);
    assert_eq!(&tmp[..n1], b"abcd");

    // read across chunk boundary
    let n2 = reader.read(&mut tmp).unwrap();
    assert_eq!(n2, 4);
    assert_eq!(&tmp[..n2], b"efgh");

    // read the rest to make sure we don't read over
    let n3 = reader.read(&mut tmp).unwrap();
    assert_eq!(n3, 3);
    assert_eq!(&tmp[..n3], b"ijk");

    // Reader exhausted — next read returns 0.
    let n4 = reader.read(&mut tmp).unwrap();
    assert_eq!(n4, 0);
}

#[test]
#[allow(static_mut_refs)]
fn test_envoy_buffers_reader_empty_chunk_skipped() {
    static mut CHUNK_BEFORE: [u8; 3] = *b"abc";
    static mut CHUNK_EMPTY: [u8; 0] = [];
    static mut CHUNK_AFTER: [u8; 3] = *b"xyz";
    let buffers = vec![
        EnvoyMutBuffer::new(unsafe { &mut CHUNK_BEFORE }),
        EnvoyMutBuffer::new(unsafe { &mut CHUNK_EMPTY }),
        EnvoyMutBuffer::new(unsafe { &mut CHUNK_AFTER }),
    ];
    let mut reader = EnvoyBuffersReader::new(buffers);
    let mut out = Vec::new();
    reader.read_to_end(&mut out).unwrap();
    assert_eq!(out, b"abcxyz");
}
