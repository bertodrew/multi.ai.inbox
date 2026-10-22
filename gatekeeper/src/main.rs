//! JSON lines worker: one `Input` object per line on stdin, one `Output`
//! object per line on stdout. The Go enrich worker keeps this process alive.

use std::io::{self, BufRead, Write};

fn main() -> io::Result<()> {
    let stdin = io::stdin();
    let mut out = io::BufWriter::new(io::stdout().lock());
    for line in stdin.lock().lines() {
        let line = line?;
        if line.trim().is_empty() {
            continue;
        }
        let reply = match serde_json::from_str::<gatekeeper::Input>(&line) {
            Ok(input) => serde_json::to_string(&gatekeeper::scan(&input)),
            Err(err) => serde_json::to_string(&serde_json::json!({ "status": "unavailable", "error": err.to_string() })),
        }
        .map_err(io::Error::other)?;
        writeln!(out, "{reply}")?;
        out.flush()?;
    }
    Ok(())
}
