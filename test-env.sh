#!/bin/bash
# Quick test to verify environment variables are accessible

echo "Testing environment variables:"
echo "OPENAI_API_KEY: ${OPENAI_API_KEY:0:20}..."
echo "AWS_ACCESS_KEY_ID_SES: ${AWS_ACCESS_KEY_ID_SES:0:10}..."
echo "AWS_SECRET_ACCESS_KEY_SES: ${AWS_SECRET_ACCESS_KEY_SES:0:10}..."
echo "AWS_REGION_SES: ${AWS_REGION_SES}"
echo ""

if [ -z "$OPENAI_API_KEY" ]; then
    echo "❌ OPENAI_API_KEY is NOT set"
else
    echo "✅ OPENAI_API_KEY is set"
fi

if [ -z "$AWS_ACCESS_KEY_ID_SES" ]; then
    echo "❌ AWS_ACCESS_KEY_ID_SES is NOT set"
else
    echo "✅ AWS_ACCESS_KEY_ID_SES is set"
fi

echo ""
echo "Now testing Go's ability to read the variable:"
go run -C /Users/robertgehrsitz/Code/lab_monitor -e 'package main; import ("fmt"; "os"); func main() { fmt.Printf("OPENAI_API_KEY from Go: %s\n", os.Getenv("OPENAI_API_KEY")[:20] + "...") }' 2>&1 || cat <<'EOF'
package main
import ("fmt"; "os")
func main() {
    key := os.Getenv("OPENAI_API_KEY")
    if key == "" {
        fmt.Println("❌ Go cannot see OPENAI_API_KEY")
    } else {
        fmt.Printf("✅ Go can see OPENAI_API_KEY: %s...\n", key[:20])
    }
}
EOF
