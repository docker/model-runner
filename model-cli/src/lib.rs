mod auth;
mod config;
mod error;
mod handlers;
mod providers;
mod router;
mod types;

use std::ffi::CStr;
use std::os::raw::{c_char, c_int};
use std::path::PathBuf;

/// C-callable entry point invoked by the Go CLI's `gateway` subcommand.
///
/// # Safety
/// `argv` must be a valid array of `argc` non-null, null-terminated C strings
/// that remains valid for the duration of this call.
#[no_mangle]
pub unsafe extern "C" fn run_gateway(argc: c_int, argv: *const *const c_char) -> c_int {
    // Rebuild the argument list from the C array.
    // argv[0] is the program name (ignored by clap when we parse manually);
    // the remaining elements are the gateway flags/values.
    let args: Vec<String> = (0..argc as usize)
        .filter_map(|i| {
            let ptr = *argv.add(i);
            if ptr.is_null() {
                None
            } else {
                CStr::from_ptr(ptr).to_str().ok().map(|s| s.to_owned())
            }
        })
        .collect();

    // Parse flags directly (skip argv[0] = program name).
    // Expected layout: ["model-cli", "--config", "<path>", ...]
    let mut config: Option<PathBuf> = None;
    let mut host = "0.0.0.0".to_string();
    let mut port: u16 = 4000;
    let mut verbose = false;

    let mut it = args.iter().skip(1).peekable();
    while let Some(arg) = it.next() {
        match arg.as_str() {
            "-c" | "--config" => {
                if let Some(val) = it.next() {
                    config = Some(PathBuf::from(val));
                }
            }
            "--host" => {
                if let Some(val) = it.next() {
                    host = val.clone();
                }
            }
            "-p" | "--port" => {
                if let Some(val) = it.next() {
                    if let Ok(n) = val.parse::<u16>() {
                        port = n;
                    }
                }
            }
            "-v" | "--verbose" => verbose = true,
            "--help" | "-h" => {
                eprintln!(concat!(
                    "Usage: model-cli gateway [OPTIONS] --config <CONFIG>\n",
                    "\n",
                    "Options:\n",
                    "  -c, --config <CONFIG>   Path to the YAML configuration file\n",
                    "      --host <HOST>        Host address [default: 0.0.0.0]\n",
                    "  -p, --port <PORT>        Port [default: 4000]\n",
                    "  -v, --verbose            Enable debug logging\n",
                    "  -h, --help               Print help",
                ));
                return 0;
            }
            _ => {}
        }
    }

    let config = match config {
        Some(p) => p,
        None => {
            eprintln!("error: --config is required");
            return 1;
        }
    };

    let rt = match tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
    {
        Ok(r) => r,
        Err(e) => {
            eprintln!("error: failed to build tokio runtime: {e}");
            return 1;
        }
    };

    rt.block_on(handlers::run_gateway_async(config, host, port, verbose));
    0
}
