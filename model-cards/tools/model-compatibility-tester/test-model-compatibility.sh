#!/bin/bash

# Model Compatibility Tester
# Tests Docker AI models to check compatibility
# Usage: ./test-model-compatibility.sh [options]

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"

# Default configuration
DEFAULT_TEST_PROMPT="Write me a 10 word poem"
RESULTS_DIR="$SCRIPT_DIR/results"
LOG_FILE="$RESULTS_DIR/test-$(date +%Y%m%d-%H%M%S).log"

# Command line options
NAMESPACE=""
MODELS=""
VARIANTS=""
TEST_PROMPT="$DEFAULT_TEST_PROMPT"

# Global hardware info
HARDWARE_TYPE=""
TOTAL_MEMORY_MB=""
GPU_MEMORY_MB=""

# Usage information
usage() {
    cat << EOF
Model Compatibility Tester

USAGE:
    $0 [OPTIONS]

OPTIONS:
    -n, --namespace NAMESPACE   Docker Hub namespace to test all repositories from
    -m, --models MODELS         Comma-separated list of models to test (default: all)
    -v, --variants VARIANTS     Comma-separated list of variants to test (default: all)
    --prompt TEXT               Test prompt (default: "$DEFAULT_TEST_PROMPT")
    -h, --help                  Show this help message

EXAMPLES:
    # Test all repositories and tags from "ai" namespace
    $0 --namespace ai

    # Test all models from local directory
    $0

    # Test specific models
    $0 --models ai/llama3.1,ai/qwen2.5

    # Test specific model with variant
    $0 --models ai/llama3.2:latest

    # Test namespace with custom prompt
    $0 --namespace ai --prompt "Hello world"

EOF
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -n|--namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            -m|--models)
                MODELS="$2"
                shift 2
                ;;
            -v|--variants)
                VARIANTS="$2"
                shift 2
                ;;
            --prompt)
                TEST_PROMPT="$2"
                shift 2
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                echo "Unknown option: $1" >&2
                usage >&2
                exit 1
                ;;
        esac
    done
}

# Logging functions
log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    
    echo "[$timestamp] [$level] $message" | tee -a "$LOG_FILE"
    
    if [[ "$level" == "ERROR" ]]; then
        echo "[$timestamp] [$level] $message" >&2
    fi
}

log_info() {
    log "INFO" "$@"
}

log_warn() {
    log "WARN" "$@"
}

log_error() {
    log "ERROR" "$@"
}

# Hardware detection
detect_hardware() {
    local total_memory_mb=0
    local gpu_memory_mb=0
    local hardware_type="unknown"
    
    # Detect total system memory
    if command -v free >/dev/null 2>&1; then
        # Linux
        total_memory_mb=$(free -m | awk '/^Mem:/ {print $2}')
        hardware_type="linux"
    elif [[ "$(uname)" == "Darwin" ]]; then
        # macOS
        local total_memory_bytes
        total_memory_bytes=$(sysctl -n hw.memsize)
        total_memory_mb=$((total_memory_bytes / 1024 / 1024))
        hardware_type="macos"
    fi
    
    # Detect GPU memory (basic detection)
    if command -v nvidia-smi >/dev/null 2>&1; then
        gpu_memory_mb=$(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits | head -1)
        hardware_type="nvidia"
    fi
    
    echo "type:$hardware_type,total_memory:$total_memory_mb,gpu_memory:$gpu_memory_mb"
}

# Parse hardware info and set global variables
parse_hardware_info() {
    local hardware_info="$1"
    
    # Parse the hardware info string: "type:macos,total_memory:131072,gpu_memory:0"
    HARDWARE_TYPE=$(echo "$hardware_info" | sed 's/.*type:\([^,]*\).*/\1/')
    TOTAL_MEMORY_MB=$(echo "$hardware_info" | sed 's/.*total_memory:\([^,]*\).*/\1/')
    GPU_MEMORY_MB=$(echo "$hardware_info" | sed 's/.*gpu_memory:\([^,]*\).*/\1/')
}

# Discover models from ai directory
discover_models() {
    local ai_dir="$1"
    
    if [[ ! -d "$ai_dir" ]]; then
        log_error "AI directory not found: $ai_dir"
        return 1
    fi
    
    find "$ai_dir" -name "*.md" -type f | sed 's|.*/||; s|\.md$||' | sed 's|^|ai/|' | sort
}

# Discover repositories from Docker Hub namespace
discover_namespace_repositories() {
    local namespace="$1"
    local page=1
    local repositories=()
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    
    echo "[$timestamp] [INFO] Discovering repositories from namespace: $namespace" >&2
    
    while true; do
        local url="https://hub.docker.com/v2/namespaces/$namespace/repositories/?page=$page&page_size=100"
        local response=$(curl -s "$url")
        
        # Extract repository names from JSON response
        local repo_names=$(echo "$response" | grep -o '"name":"[^"]*"' | sed 's/"name":"//g' | sed 's/"//g')
        
        if [[ -z "$repo_names" ]]; then
            break
        fi
        
        repositories+=($repo_names)
        
        # Check if there's a next page
        local next_page=$(echo "$response" | grep -o '"next":"[^"]*"')
        if [[ -z "$next_page" ]]; then
            break
        fi
        
        ((page++))
    done
    
    if [[ ${#repositories[@]} -gt 0 ]]; then
        echo "[$timestamp] [INFO] Found ${#repositories[@]} repositories in namespace $namespace" >&2
        printf '%s\n' "${repositories[@]}"
    else
        echo "[$timestamp] [INFO] Found 0 repositories in namespace $namespace" >&2
    fi
}

# Get all tags for a repository (except latest)
get_repository_tags() {
    local namespace="$1"
    local repository="$2"
    local page=1
    local tags=()
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    
    echo "[$timestamp] [INFO] Getting tags for $namespace/$repository" >&2
    
    while true; do
        local url="https://hub.docker.com/v2/repositories/$namespace/$repository/tags/?page=$page&page_size=100"
        local response=$(curl -s "$url")
        
        # Extract tag names from JSON response, excluding "latest"
        local tag_names=$(echo "$response" | grep -o '"name":"[^"]*"' | sed 's/"name":"//g' | sed 's/"//g' | grep -v "^latest$")
        
        if [[ -z "$tag_names" ]]; then
            break
        fi
        
        tags+=($tag_names)
        
        # Check if there's a next page
        local next_page=$(echo "$response" | grep -o '"next":"[^"]*"')
        if [[ -z "$next_page" ]]; then
            break
        fi
        
        ((page++))
    done
    
    if [[ ${#tags[@]} -gt 0 ]]; then
        echo "[$timestamp] [INFO] Found ${#tags[@]} tags for $namespace/$repository (excluding latest)" >&2
        printf '%s\n' "${tags[@]}"
    else
        echo "[$timestamp] [INFO] Found 0 tags for $namespace/$repository (excluding latest)" >&2
    fi
}

# Get model variants (fallback for non-namespace mode)
get_model_variants() {
    local model="$1"
    
    # For now, return common quantization patterns
    crane ls "$model"
}

# Initialize CSV results file
init_csv_results() {
    local csv_file="$1"
    
    if [[ ! -f "$csv_file" ]]; then
        echo "timestamp,model,variant,hardware_type,total_memory_mb,gpu_memory_mb,status,duration_seconds,error_type,error_message" > "$csv_file"
    fi
}

# Record test result
record_result() {
    local model="$1"
    local variant="$2"
    local status="$3"
    local duration="${4:-0}"
    local error_type="${5:-}"
    local error_message="${6:-}"
    
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local csv_file="$RESULTS_DIR/results.csv"
    
    # Escape commas and quotes in error message
    error_message=$(echo "$error_message" | sed 's/,/;/g' | sed 's/"/'"'"'/g')
    
    echo "$timestamp,$model,$variant,$HARDWARE_TYPE,$TOTAL_MEMORY_MB,$GPU_MEMORY_MB,$status,$duration,$error_type,$error_message" >> "$csv_file"
    
    log_info "Result recorded: $model:$variant -> $status"
}

# Test a single model variant
test_model_variant() {
    local full_model_name="$1"
    local start_time=$(date +%s)
    local model=$(echo "$full_model_name" | cut -d: -f1)
    local variant=$(echo "$full_model_name" | cut -d: -f2)
    
    log_info "Testing: $full_model_name"
    
    # Step 1: Pull the model
    log_info "Pulling model: $full_model_name"
    if ! docker model pull "$full_model_name" >/dev/null 2>&1; then
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        record_result "$model" "$variant" "PULL_FAILED" "$duration" "PULL_ERROR" "Failed to pull model"
        return 1
    fi
    
    # Step 2: Run the test
    log_info "Running test with prompt: $TEST_PROMPT"
    local test_output_file="/tmp/model_test_output_$$"
    local test_error_file="/tmp/model_test_error_$$"
    
    if docker model run "$full_model_name" "$TEST_PROMPT" > "$test_output_file" 2> "$test_error_file"; then
        # Success
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        local output=$(head -c 100 "$test_output_file" | tr '\n' ' ')
        
        record_result "$model" "$variant" "SUCCESS" "$duration" "" "Output: $output"
        log_info "✅ SUCCESS: $full_model_name (${duration}s)"
        
        # Cleanup temp files
        rm -f "$test_output_file" "$test_error_file"
        
        # Step 3: Cleanup model
        docker model rm "$full_model_name" >/dev/null 2>&1 || true
        
        return 0
    else
        # Failure - analyze error
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        local error_output=$(cat "$test_error_file" 2>/dev/null || echo "No error output")
        local error_type="RUNTIME_ERROR"
        
        # Classify error type
        if echo "$error_output" | grep -qi "out of memory\|oom\|memory\|cuda.*memory\|insufficient.*memory"; then
            error_type="MEMORY_ERROR"
        elif echo "$error_output" | grep -qi "not found\|no such"; then
            error_type="MODEL_NOT_FOUND"
        fi
        
        record_result "$model" "$variant" "FAILED" "$duration" "$error_type" "$error_output"
        log_error "❌ FAILED: $full_model_name ($error_type, $error_output, ${duration}s)"
        
        # Cleanup temp files
        rm -f "$test_output_file" "$test_error_file"
        
        # Step 3: Cleanup model (attempt even if test failed)
        docker model rm "$full_model_name" >/dev/null 2>&1 || true
        
        return 1
    fi
}

# Initialize results directory
init_results_dir() {
    mkdir -p "$RESULTS_DIR"
    init_csv_results "$RESULTS_DIR/results.csv"
    
    # Create log file
    touch "$LOG_FILE"
}

# Main function
main() {
    parse_args "$@"
    
    # Initialize
    init_results_dir
    
    log_info "Starting Model Compatibility Tester"
    log_info "Results directory: $RESULTS_DIR"
    
    # Check if Docker is available
    if ! command -v docker >/dev/null 2>&1; then
        log_error "Docker is not installed or not in PATH"
        exit 1
    fi
    
    # Check if docker model command is available
    if ! docker model --help >/dev/null 2>&1; then
        log_error "Docker model command not available. Please ensure Docker Desktop with AI features is installed."
        exit 1
    fi
    
    # Get hardware info
    local hardware_info
    hardware_info=$(detect_hardware)
    parse_hardware_info "$hardware_info"
    log_info "Hardware configuration: $hardware_info"
    
    # Discover models to test
    local models_to_test=()
    
    if [[ -n "$NAMESPACE" ]]; then
        # Use Docker Hub API to discover repositories and tags
        log_info "Using namespace discovery for: $NAMESPACE"
        
        while IFS= read -r repository; do
            if [[ -n "$repository" ]]; then
                while IFS= read -r tag; do
                    if [[ -n "$tag" ]]; then
                        models_to_test+=("$NAMESPACE/$repository:$tag")
                    fi
                done < <(get_repository_tags "$NAMESPACE" "$repository")
            fi
        done < <(discover_namespace_repositories "$NAMESPACE")
        
    elif [[ -n "$MODELS" ]]; then
        IFS=',' read -ra models_to_test <<< "$MODELS"
    else
        while IFS= read -r model; do
            models_to_test+=("$model")
        done < <(discover_models "$PROJECT_ROOT/ai")
    fi
    
    log_info "Found ${#models_to_test[@]} models to test: ${models_to_test[*]}"
    
    # Test each model
    local total_tests=0
    local successful_tests=0
    local failed_tests=0
    
    for model in "${models_to_test[@]}"; do
        log_info "Processing model: $model"
        
        # Check if model already includes a variant (contains :)
        if [[ "$model" == *":"* ]]; then
            ((total_tests++))
            
            # Run the test
            if test_model_variant "$model"; then
                ((successful_tests++))
            else
                ((failed_tests++))
            fi
            
            # Brief pause between tests
            sleep 2
        else
            # Model without variant, discover variants
            local variants_to_test=()
            if [[ -n "$VARIANTS" ]]; then
                IFS=',' read -ra variants_to_test <<< "$VARIANTS"
            else
                while IFS= read -r variant; do
                    variants_to_test+=("$variant")
                done < <(get_model_variants "$model")
            fi
            
            log_info "Found ${#variants_to_test[@]} variants for $model: ${variants_to_test[*]}"
            
            # Test each variant
            for variant in "${variants_to_test[@]}"; do
                local full_model_name="${model}:${variant}"

                ((total_tests++))
                
                # Run the test
                if test_model_variant "$full_model_name"; then
                    ((successful_tests++))
                else
                    ((failed_tests++))
                fi
                
                # Brief pause between tests
                sleep 2
            done
        fi
    done
    
    # Final summary
    log_info "Testing completed!"
    log_info "Total tests: $total_tests"
    log_info "Successful: $successful_tests"
    log_info "Failed: $failed_tests"
    
    if [[ $total_tests -gt 0 ]]; then
        local success_rate=$((successful_tests * 100 / total_tests))
        log_info "Success rate: ${success_rate}%"
    else
        log_info "No tests were executed"
    fi
    
    log_info "Results saved to: $RESULTS_DIR/results.csv"
    log_info "Log file: $LOG_FILE"
}

# Run main function with all arguments
main "$@"
