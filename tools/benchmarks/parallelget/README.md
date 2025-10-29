# ParallelGet Benchmark Tool

A command-line benchmarking tool that compares the performance of standard HTTP GET requests against parallelized requests using the `transport/parallel` package.

## Features

- **Performance Comparison**: Downloads the same URL twice (standard vs parallel) and compares timing
- **Response Validation**: Ensures both downloads produce identical results (byte-for-byte comparison)
- **Configurable Parameters**: Adjustable chunk size and concurrency settings
- **Detailed Metrics**: Reports download speeds, timing differences, and performance improvements
- **Dynamic Progress Display**: Shows real-time progress bars during downloads with percentage and byte counts
- **Clean Output**: User-friendly performance summary with emojis and clear formatting

## Usage

```bash
go run ./tools/benchmarks/parallelget <url> [flags]
```

or

```bash
go build ./tools/benchmarks/parallelget
./parallelget <url> [flags]
```

### Arguments

- `<url>`: The HTTP URL to benchmark (required)

### Flags

- `--chunk-size int`: Minimum chunk size in bytes for parallelization (default: 1MB)
- `--max-concurrent uint`: Maximum concurrent requests for parallel transport (default: 4)
- `-h, --help`: Show help information

### Examples

```bash
# Basic usage
./parallelget https://example.com/large-file.zip

# Custom chunk size (512KB) and higher concurrency
./parallelget https://example.com/large-file.zip --chunk-size 524288 --max-concurrent 8

# Small chunk size for testing with smaller files
./parallelget https://httpbin.org/bytes/10485760 --chunk-size 262144 --max-concurrent 6
```

## Output

The tool provides detailed output including:

1. **Configuration**: Shows the chunk size and concurrency settings
2. **Progress**: Real-time updates for each benchmark phase
3. **Individual Results**: Download speed and timing for each approach
4. **Validation**: Confirms that both downloads produced identical content
5. **Performance Summary**: 
   - Speedup factor (e.g., "3.2x faster")
   - Time saved/penalty
   - Detailed timing breakdown

### Sample Output

```
Benchmarking HTTP GET performance for: https://example.com/large-file.zip
Configuration: chunk-size=1048576 bytes, max-concurrent=4

Running non-parallel benchmark...
  Progress: [██████████████████████████████] 100.0% (10485760/10485760 bytes)
✓ Non-parallel: 10485760 bytes in 2.1s (4.76 MB/s)
Running parallel benchmark...
  Progress: [██████████████████████████████] 100.0% (10485760/10485760 bytes)
✓ Parallel: 10485760 bytes in 650ms (15.38 MB/s)
Validating response consistency...
✓ Responses match perfectly

============================================================
PERFORMANCE COMPARISON
============================================================
🚀 Parallel was 3.23x faster than non-parallel
⏱️  Time saved: 1.45s (69.0%)

Detailed timing:
  Non-parallel: 2.1s
  Parallel:     650ms
  Difference:   -1.45s
```

## How It Works

1. **Non-Parallel Benchmark**: Uses `net/http.DefaultClient` with `net/http.DefaultTransport`
2. **Parallel Benchmark**: Uses `net/http.DefaultClient` with `transport/parallel.ParallelTransport` wrapping `net/http.DefaultTransport`
3. **Response Storage**: Both responses are written to temporary files for validation
4. **Validation**: Performs byte-by-byte comparison to ensure identical content
5. **Cleanup**: Automatically removes temporary files after completion

## Notes

- The tool requires the server to support HTTP range requests (`Accept-Ranges: bytes`) for parallel downloads to work
- If the server doesn't support range requests or the file is too small, the parallel transport will automatically fall back to a single request
- Temporary files are automatically cleaned up, even if the tool exits unexpectedly
- The tool validates that both downloads produce identical results before reporting performance metrics

## Use Cases

- **Performance Testing**: Evaluate the effectiveness of parallel downloads for different URLs
- **Configuration Tuning**: Find optimal chunk size and concurrency settings for specific servers or file types  
- **Server Compatibility**: Test whether servers properly support range requests
- **Network Optimization**: Understand the impact of parallel downloads on different network conditions
