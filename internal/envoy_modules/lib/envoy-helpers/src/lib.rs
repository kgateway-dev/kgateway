#![deny(clippy::unwrap_used, clippy::expect_used)]

pub mod http;

use envoy_proxy_dynamic_modules_rust_sdk::EnvoyMutBuffer;

/// Adapts a `Vec<EnvoyMutBuffer>` into a `std::io::Read` implementation,
/// concatenating each buffer chunk in order.
pub struct EnvoyBuffersReader<'a> {
    buffers: Vec<EnvoyMutBuffer<'a>>,
    chunk_idx: usize,
    offset: usize,
}

impl<'a> EnvoyBuffersReader<'a> {
    pub fn new(buffers: Vec<EnvoyMutBuffer<'a>>) -> Self {
        Self {
            buffers,
            chunk_idx: 0,
            offset: 0,
        }
    }
}

impl std::io::Read for EnvoyBuffersReader<'_> {
    fn read(&mut self, buf: &mut [u8]) -> std::io::Result<usize> {
        let mut filled = 0;
        while filled < buf.len() && self.chunk_idx < self.buffers.len() {
            let chunk = self.buffers[self.chunk_idx].as_slice();
            let remaining = &chunk[self.offset..];
            if remaining.is_empty() {
                self.chunk_idx += 1;
                self.offset = 0;
                continue;
            }
            let n = remaining.len().min(buf.len() - filled);
            buf[filled..filled + n].copy_from_slice(&remaining[..n]);
            self.offset += n;
            filled += n;
            if self.offset >= chunk.len() {
                self.chunk_idx += 1;
                self.offset = 0;
            }
        }
        Ok(filled)
    }
}

#[cfg(test)]
mod tests;
