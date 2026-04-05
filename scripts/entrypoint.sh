#!/bin/bash

set -e

echo "🎵 TLMSC Entrypoint - Initializing..."

# Use HOME env var or default to /tmp for config
CONFIG_HOME="${HOME:-.}"
mkdir -p "$CONFIG_HOME/.config/streamrip" /data/staging

# Generate default streamrip config
echo "📝 Generating streamrip configuration..."
rip config reset -y > /dev/null 2>&1 || echo "Warning: Could not reset streamrip config"

# Patch Qobuz credentials if provided
if [ -n "$QOBUZ_EMAIL" ] && [ -n "$QOBUZ_PASSWORD" ]; then
    echo "🔐 Setting Qobuz credentials..."
    
    # Patch the config file using awk to avoid busybox sed limitations with long values
    CONFIG_FILE="$CONFIG_HOME/.config/streamrip/config.toml"
    awk -v val="$QOBUZ_EMAIL" '/^email_or_userid = /{print "email_or_userid = \"" val "\""; next}1' "$CONFIG_FILE" > "$CONFIG_FILE.tmp" && mv "$CONFIG_FILE.tmp" "$CONFIG_FILE"
    awk -v val="$QOBUZ_PASSWORD" '/^password_or_token = /{print "password_or_token = \"" val "\""; next}1' "$CONFIG_FILE" > "$CONFIG_FILE.tmp" && mv "$CONFIG_FILE.tmp" "$CONFIG_FILE"
    awk '/^use_auth_token = /{print "use_auth_token = false"; next}1' "$CONFIG_FILE" > "$CONFIG_FILE.tmp" && mv "$CONFIG_FILE.tmp" "$CONFIG_FILE"
    
    echo "✅ Qobuz configured"
else
    if [ -z "$QOBUZ_EMAIL" ] && [ -z "$QOBUZ_PASSWORD" ]; then
        echo "⚠️  Qobuz credentials not provided (QOBUZ_EMAIL and QOBUZ_PASSWORD)"
    else
        echo "⚠️  Incomplete Qobuz credentials (both email and password required)"
    fi
fi

# Patch Deezer ARL if provided
if [ -n "$DEEZER_ARL" ]; then
    echo "🔐 Setting Deezer credentials..."
    
    # Patch the config file using awk to avoid busybox sed limitations with long values
    CONFIG_FILE="$CONFIG_HOME/.config/streamrip/config.toml"
    awk -v val="$DEEZER_ARL" '/^arl = /{print "arl = \"" val "\""; next}1' "$CONFIG_FILE" > "$CONFIG_FILE.tmp" && mv "$CONFIG_FILE.tmp" "$CONFIG_FILE"
    awk '/^use_deezloader = /{print "use_deezloader = false"; next}1' "$CONFIG_FILE" > "$CONFIG_FILE.tmp" && mv "$CONFIG_FILE.tmp" "$CONFIG_FILE"
    
    echo "✅ Deezer configured"
else
    echo "⚠️  Deezer ARL not provided (DEEZER_ARL environment variable)"
fi

# Update download folder to staging path
echo "📁 Setting download folder to /data/staging..."
awk '/^folder = /{print "folder = \"/data/staging\""; next}1' "$CONFIG_HOME/.config/streamrip/config.toml" > "$CONFIG_HOME/.config/streamrip/config.toml.tmp" && mv "$CONFIG_HOME/.config/streamrip/config.toml.tmp" "$CONFIG_HOME/.config/streamrip/config.toml"

# Verify config is valid
echo "✅ Streamrip configuration generated at $CONFIG_HOME/.config/streamrip/config.toml"

# Check credential status
CRED_STATUS=""
if [ -n "$QOBUZ_EMAIL" ] && [ -n "$QOBUZ_PASSWORD" ]; then
    CRED_STATUS="$CRED_STATUS Qobuz"
fi
if [ -n "$DEEZER_ARL" ]; then
    CRED_STATUS="$CRED_STATUS Deezer"
fi

if [ -z "$CRED_STATUS" ]; then
    echo ""
    echo "⚠️  WARNING: No streaming service credentials configured!"
    echo "   Set at least one of:"
    echo "   - QOBUZ_EMAIL and QOBUZ_PASSWORD"
    echo "   - DEEZER_ARL"
    echo ""
else
    echo ""
    echo "✅ Configured services:$CRED_STATUS"
    echo ""
fi

echo "✅ Directories initialized"
echo "🚀 Starting TLMSC bot..."
echo ""

# Execute the main command (passed as arguments)
exec "$@"
