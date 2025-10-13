# Troubleshooting Guide

## Environment Variables Not Being Read

### Problem
Running `go run ./cmd/labmonitor -dry-run` gives:
```
init llm client: OPENAI_API_KEY is not set
```

Even though `printenv` shows the variable is set.

### Solution

#### Option 1: Ensure Variables Are Exported
Make sure your environment variables are **exported**, not just set:

```bash
# Wrong (variable not exported to child processes)
OPENAI_API_KEY="sk-..."

# Correct (variable exported)
export OPENAI_API_KEY="sk-..."
```

Check if a variable is exported:
```bash
# This will only work if the variable is exported
echo $OPENAI_API_KEY
```

#### Option 2: Use an .envrc File
1. Copy the example file:
   ```bash
   cp .envrc.example .envrc
   ```

2. Edit `.envrc` with your actual credentials:
   ```bash
   nano .envrc  # or vim, or your favorite editor
   ```

3. Source it before running:
   ```bash
   source .envrc
   go run ./cmd/labmonitor -dry-run
   ```

#### Option 3: Inline Environment Variables
Pass variables directly to the command:
```bash
OPENAI_API_KEY="your-key" \
AWS_ACCESS_KEY_ID_SES="your-aws-key" \
AWS_SECRET_ACCESS_KEY_SES="your-aws-secret" \
go run ./cmd/labmonitor -dry-run
```

#### Option 4: Use the Helper Script
```bash
# Make sure variables are exported first
export OPENAI_API_KEY="your-key"
export AWS_ACCESS_KEY_ID_SES="your-aws-key"
export AWS_SECRET_ACCESS_KEY_SES="your-aws-secret"

# Then run the helper
./run-dry-run.sh
```

### Verify Environment Variables Are Accessible

Run this test:
```bash
go run -e 'package main; import ("fmt"; "os"); func main() { if k := os.Getenv("OPENAI_API_KEY"); k == "" { fmt.Println("NOT SET") } else { fmt.Println("SET:", k[:10]+"...") } }'
```

Or use this simpler test:
```bash
bash -c 'echo "OPENAI_API_KEY in subshell: ${OPENAI_API_KEY:0:20}..."'
```

If the subshell can't see it, neither can Go.

### Common Issues

1. **Variable not exported**: Use `export` before the variable assignment
2. **Running from wrong directory**: Make sure you're in `/Users/robertgehrsitz/Code/lab_monitor`
3. **Shell configuration**: Variables set in `~/.zshrc` or `~/.bashrc` need to use `export`

### Shell-Specific Notes

**For zsh (macOS default):**
Add to `~/.zshrc`:
```bash
export OPENAI_API_KEY="your-key"
export AWS_ACCESS_KEY_ID_SES="your-key"
export AWS_SECRET_ACCESS_KEY_SES="your-secret"
export AWS_REGION_SES="us-east-2"
```

Then reload:
```bash
source ~/.zshrc
```

**For bash:**
Add to `~/.bash_profile` or `~/.bashrc`:
```bash
export OPENAI_API_KEY="your-key"
export AWS_ACCESS_KEY_ID_SES="your-key"
export AWS_SECRET_ACCESS_KEY_SES="your-secret"
export AWS_REGION_SES="us-east-2"
```

Then reload:
```bash
source ~/.bash_profile
```

## API Key Security

**IMPORTANT:** Never commit API keys to git!

- `.envrc` is in `.gitignore`
- Don't paste API keys in public places
- Rotate keys if they're exposed
- Use OpenAI's API key management: https://platform.openai.com/api-keys

If you accidentally exposed your key:
1. Go to https://platform.openai.com/api-keys
2. Revoke the exposed key
3. Generate a new key
4. Update your environment variables

## Other Common Issues

### "config.yaml not found"
Make sure you're running from the project directory:
```bash
cd /Users/robertgehrsitz/Code/lab_monitor
```

### Timeout errors
The first run might take a while as it:
1. Fetches data from ThingSpeak
2. Sends request to OpenAI (can take 10-30 seconds)
3. Generates the report

Be patient on the first run.

### AWS SES errors (when not using -dry-run)
- Ensure your sender email is verified in AWS SES
- Check that you're using the correct region (default: us-east-2)
- Verify AWS credentials are correct
