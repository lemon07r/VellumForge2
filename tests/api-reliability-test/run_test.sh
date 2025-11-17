#!/bin/bash
set -e

# API Reliability Stress Test Runner
# Tests streaming reliability under load for different API providers

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/../.."

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "═══════════════════════════════════════════════════════════════════════════"
echo "  API Reliability Stress Test"
echo "═══════════════════════════════════════════════════════════════════════════"
echo ""

# Check if binary exists
if [ ! -f "bin/vellumforge2" ]; then
    echo -e "${YELLOW}Binary not found. Building...${NC}"
    make build
fi

# Function to run test for a provider
run_provider_test() {
    local provider=$1
    local config="tests/api-reliability-test/config.${provider}.toml"
    local env_file="tests/api-reliability-test/.env.${provider}"
    
    echo -e "${BLUE}Testing ${provider} provider...${NC}"
    echo "  Config: $config"
    echo "  Env: $env_file"
    echo ""
    
    if [ ! -f "$env_file" ]; then
        echo -e "${RED}ERROR: $env_file not found!${NC}"
        echo "  Copy .env.${provider}.example to .env.${provider} and add your API key"
        return 1
    fi
    
    # Run the test
    timeout 1800 ./bin/vellumforge2 run --config "$config" --env-file "$env_file" 2>&1 | \
        tee "tests/api-reliability-test/test_${provider}.log"
    
    echo ""
}

# Parse arguments
PROVIDERS="${1:-both}"

case "$PROVIDERS" in
    nahcrof)
        run_provider_test nahcrof
        ;;
    chutes)
        run_provider_test chutes
        ;;
    both)
        echo "Testing both providers (this will take ~10-15 minutes total)"
        echo ""
        run_provider_test nahcrof
        echo ""
        echo "═══════════════════════════════════════════════════════════════════════════"
        echo ""
        run_provider_test chutes
        ;;
    *)
        echo "Usage: $0 [nahcrof|chutes|both]"
        echo ""
        echo "Examples:"
        echo "  $0              # Test both providers"
        echo "  $0 nahcrof      # Test only nahcrof"
        echo "  $0 chutes       # Test only chutes"
        exit 1
        ;;
esac

echo ""
echo "═══════════════════════════════════════════════════════════════════════════"
echo -e "${GREEN}Test complete! Running analysis...${NC}"
echo "═══════════════════════════════════════════════════════════════════════════"
echo ""

# Run analysis
python3 tests/api-reliability-test/analyze_results.py

echo ""
echo -e "${GREEN}Analysis complete! Check tests/api-reliability-test/ for results.${NC}"
