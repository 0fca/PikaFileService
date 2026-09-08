#!/bin/bash
# Interactive configuration wizard for PikaFileService
# Called by postinst during .deb package installation, or can be run standalone.
#
# This script guides the user through:
#   1. Choosing installation mode (local-only vs PikaCloud)
#   2. If PikaCloud: OAuth2 device flow login -> bucket fetching -> directory-to-bucket mapping
#   3. If local-only: configuring dstPath and watched folders
#   4. Writing config.json

set -euo pipefail

# ── Ensure interactive input ─────────────────────
# When run from dpkg postinst, stdin is not a terminal.
# Reopen stdin from /dev/tty so all read calls work interactively.
if [ ! -t 0 ] && [ -e /dev/tty ]; then
    exec < /dev/tty
fi

CONFIG_DIR="/opt/pikafileservice"
CONFIG_FILE="${CONFIG_DIR}/config.json"
BINARY="${CONFIG_DIR}/pikafileservice"
LOG_DIR="/var/log/pikafileservice"

# ── PikaCloud defaults ──────────────────────────
DEFAULT_PIKACLOUD_BASE_URL="https://api-core.lukas-bownik.net"
DEFAULT_IDP_URL="https://idp.lukas-bownik.net"
DEFAULT_REALM="global"
DEFAULT_CLIENT_ID="pikafileservice"
DEFAULT_SCOPES="email,profile"
DEFAULT_TOKEN_FILE="/opt/pikafileservice/.oauth2_token.json"
DEFAULT_TIMEOUT="30"
DEFAULT_RETRY_COUNT="3"

# ─────────────────────────────────────────────────
# Helpers
# ─────────────────────────────────────────────────
print_banner() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════╗"
    echo "║        PikaFileService Configuration Wizard           ║"
    echo "╚═══════════════════════════════════════════════════════╝"
    echo ""
}

ask_yes_no() {
    local prompt="$1"
    local default="${2:-n}"
    local yn
    while true; do
        if [ "$default" = "y" ]; then
            echo -n "${prompt} [Y/n]: " >&2
            read -r yn
            yn="${yn:-y}"
        else
            echo -n "${prompt} [y/N]: " >&2
            read -r yn
            yn="${yn:-n}"
        fi
        case "$yn" in
            [Yy]*) return 0 ;;
            [Nn]*) return 1 ;;
            *) echo "  Please answer y or n." >&2 ;;
        esac
    done
}

ask_input() {
    local prompt="$1"
    local default="${2:-}"
    local value
    if [ -n "$default" ]; then
        echo -n "${prompt} [${default}]: " >&2
        read -r value
        echo "${value:-$default}"
    else
        while true; do
            echo -n "${prompt}: " >&2
            read -r value
            if [ -n "$value" ]; then
                echo "$value"
                return
            fi
            echo "  Value cannot be empty." >&2
        done
    fi
}

validate_path() {
    local path="$1"
    if [[ "$path" != /* ]]; then
        echo "  ERROR: Path must be absolute (start with /)." >&2
        return 1
    fi
    return 0
}

# ─────────────────────────────────────────────────
# Step 1: Choose installation mode
# ─────────────────────────────────────────────────
choose_mode() {
    echo "PikaFileService supports two modes:" >&2
    echo "" >&2
    echo "  1) Local sync only" >&2
    echo "     Watch directories and replicate changes to a local destination path." >&2
    echo "" >&2
    echo "  2) PikaCloud sync" >&2
    echo "     Same as local sync, plus upload files to PikaCloud storage." >&2
    echo "     Requires a PikaCloud account and OAuth2 authentication." >&2
    echo "" >&2

    local choice
    while true; do
        echo -n "Choose mode [1/2]: " >&2
        read -r choice
        case "$choice" in
            1) echo "local"; return ;;
            2) echo "cloud"; return ;;
            *) echo "  Please enter 1 or 2." >&2 ;;
        esac
    done
}

# ─────────────────────────────────────────────────
# Step 2a: Local-only configuration
# ─────────────────────────────────────────────────
configure_local() {
    echo ""
    echo "── Local Sync Configuration ──────────────────────────"
    echo ""

    # Destination path
    local dst_path
    while true; do
        dst_path=$(ask_input "Destination path (where synced files are written)")
        if validate_path "$dst_path"; then
            break
        fi
    done

    # Working directory (base for relative path calculation)
    local work_dir
    while true; do
        work_dir=$(ask_input "Working directory (common root of watched folders)" "$dst_path")
        if validate_path "$work_dir"; then
            break
        fi
    done

    # Folders to watch
    echo ""
    echo "Enter directories to watch (one per line)."
    echo "Press Enter on an empty line when done."
    echo ""
    local folders=()
    while true; do
        local folder
        echo -n "  Watch folder (empty to finish): " >&2
        read -r folder
        if [ -z "$folder" ]; then
            if [ ${#folders[@]} -eq 0 ]; then
                echo "  You must add at least one folder."
                continue
            fi
            break
        fi
        if validate_path "$folder"; then
            folders+=("$folder")
            echo "    ✓ Added: $folder"
        fi
    done

    # Build JSON
    write_local_config "$dst_path" "$work_dir" "${folders[@]}"
}

write_local_config() {
    local dst_path="$1"
    local work_dir="$2"
    shift 2
    local folders=("$@")

    # Build folders JSON array
    local folders_json="["
    local first=true
    for f in "${folders[@]}"; do
        if [ "$first" = true ]; then
            first=false
        else
            folders_json+=","
        fi
        folders_json+="\"${f}\""
    done
    folders_json+="]"

    cat > "$CONFIG_FILE" << EOF
{
    "folders": ${folders_json},
    "workDir": "${work_dir}",
    "dstPath": "${dst_path}"
}
EOF
    chmod 640 "$CONFIG_FILE"
    echo ""
    echo "  ✓ Configuration written to ${CONFIG_FILE}"
}

# ─────────────────────────────────────────────────
# Step 2b: PikaCloud configuration
# ─────────────────────────────────────────────────
configure_cloud() {
    echo ""
    echo "── PikaCloud Sync Configuration ──────────────────────"
    echo ""

    # PikaCloud API base URL
    local base_url
    base_url=$(ask_input "PikaCloud API base URL" "$DEFAULT_PIKACLOUD_BASE_URL")
    base_url="${base_url%/}"

    # OAuth2 settings — sensible defaults for the official PikaCloud instance
    echo ""
    echo "OAuth2 Device Flow authentication is required."
    echo "Default values are set for the official PikaCloud instance."
    echo ""

    local issuer_url
    issuer_url=$(ask_input "OAuth2 Issuer URL (IdP)" "$DEFAULT_IDP_URL")
    issuer_url="${issuer_url%/}"

    local realm
    realm=$(ask_input "OAuth2 Realm" "$DEFAULT_REALM")

    local client_id
    client_id=$(ask_input "OAuth2 Client ID" "$DEFAULT_CLIENT_ID")

    local scopes
    scopes=$(ask_input "OAuth2 Scopes (comma-separated)" "$DEFAULT_SCOPES")

    local token_file
    token_file=$(ask_input "Token cache file path" "$DEFAULT_TOKEN_FILE")

    local timeout
    timeout=$(ask_input "HTTP request timeout (seconds)" "$DEFAULT_TIMEOUT")

    local retry_count
    retry_count=$(ask_input "Upload retry count" "$DEFAULT_RETRY_COUNT")

    # Attempt device flow login
    echo ""
    echo "── PikaCloud Authentication ─────────────────────────"
    echo ""
    echo "We will now start the OAuth2 Device Flow to authenticate"
    echo "with PikaCloud. The service binary will handle the login."
    echo ""

    if ! ask_yes_no "Proceed with PikaCloud login now?" "y"; then
        echo ""
        echo "  Skipping login. You can authenticate later by running:"
        echo "    sudo /opt/pikafileservice/pikafileservice -c ${CONFIG_FILE} --auth-only"
        echo "  After authenticating, re-run this script to continue configuration, but the service won't work for now"
        echo ""
        exit 0
    fi

    # Write a temporary config with OAuth2 settings so the binary can perform device flow
    local tmp_config
    tmp_config=$(mktemp /tmp/pfs_config_XXXXXX.json)

    # Convert comma-separated scopes to JSON array
    local scopes_json
    scopes_json=$(echo "$scopes" | sed 's/,/","/g')
    scopes_json="[\"${scopes_json}\"]"

    cat > "$tmp_config" << EOF
{
    "workDir": "/tmp",
    "pikaCloud": {
        "baseUrl": "${base_url}",
        "bucketId": "",
        "timeout": ${timeout},
        "retryCount": ${retry_count},
        "oauth2": {
            "issuerUrl": "${issuer_url}",
            "realm": "${realm}",
            "clientId": "${client_id}",
            "scopes": ${scopes_json},
            "tokenFile": "${token_file}"
        }
    }
}
EOF

    echo "  Starting device flow authentication..."
    echo ""
    echo "  The binary will perform OAuth2 Device Flow authentication."
    echo "  It will print a verification URL and a user code below."
    echo "  Open the URL in your browser, enter the code, and authorize."
    echo ""
    echo "────────────────────────────────────────────────────────"

    # Run --auth-only in foreground — it prints the URL/code, polls for
    # authorization, saves the token, then exits on its own.
    # Timeout after 5 minutes if user never authorizes.
    local login_success=false
    local auth_pid

    "$BINARY" -c "$tmp_config" --auth-only &
    auth_pid=$!

    read -r _unused

    # Give the binary a moment to finish writing the token file
    sleep 2

    # Check if the binary is still running (polling may still be in progress)
    if kill -0 "$auth_pid" 2>/dev/null; then
        # Binary still polling — wait a few more seconds for it to pick up the auth
        echo "  Waiting for the binary to complete token exchange..."
        local waited=0
        while kill -0 "$auth_pid" 2>/dev/null && [ $waited -lt 15 ]; do
            sleep 1
            waited=$((waited + 1))
        done
        # If still running after 15s, kill it
        if kill -0 "$auth_pid" 2>/dev/null; then
            kill "$auth_pid" 2>/dev/null || true
            wait "$auth_pid" 2>/dev/null || true
        fi
    else
        wait "$auth_pid" 2>/dev/null || true
    fi

    if [ -f "$token_file" ]; then
        echo ""
        echo "  ✓ Authentication successful! Token cached at ${token_file}"
        login_success=true
    fi

    if [ "$login_success" = false ]; then
        echo ""
        echo "  ⚠ Authentication may not have completed."
        if ! ask_yes_no "  Continue with configuration anyway?" "y"; then
            rm -f "$tmp_config"
            echo "  Aborted. Re-run configuration with: sudo ${CONFIG_DIR}/configure.sh"
            exit 1
        fi
    fi

    # If login succeeded, use the binary's --fetch-buckets mode to retrieve buckets.
    # This reuses the cached token via the same OAuth2 connector code path.
    local buckets_json=""
    if [ "$login_success" = true ]; then
        echo ""
        echo "── Fetching Buckets ────────────────────────────────"
        echo ""

        local fetch_output
        if fetch_output=$("$BINARY" -c "$tmp_config" --fetch-buckets 2>/dev/null); then
            # Validate JSON
            if echo "$fetch_output" | python3 -c "import sys,json; json.load(sys.stdin)" 2>/dev/null; then
                buckets_json="$fetch_output"
                echo "  ✓ Buckets retrieved successfully."
            else
                echo "  ⚠ Bucket response was not valid JSON. You can configure bucket IDs manually."
            fi
        else
            echo "  ⚠ Failed to retrieve buckets. You can configure bucket IDs manually."
        fi
    fi

    rm -f "$tmp_config"

    # Now configure directory mapping
    configure_cloud_directories "$base_url" "$issuer_url" "$realm" "$client_id" "$scopes" "$token_file" "$timeout" "$retry_count" "$buckets_json"
}

configure_cloud_directories() {
    local base_url="$1"
    local issuer_url="$2"
    local realm="$3"
    local client_id="$4"
    local scopes="$5"
    local token_file="$6"
    local timeout="$7"
    local retry_count="$8"
    local buckets_json="$9"

    echo ""
    echo "── Directory & Bucket Mapping ─────────────────────────"
    echo ""

    # Parse and display available buckets
    local bucket_ids=()
    local bucket_names=()

    if [ -n "$buckets_json" ]; then
        local num_buckets
        num_buckets=$(echo "$buckets_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d))" 2>/dev/null || echo "0")

        if [ "$num_buckets" -gt 0 ]; then
            echo "  Available buckets:"
            echo ""
            for i in $(seq 0 $((num_buckets - 1))); do
                local bid bname benc
                bid=$(echo "$buckets_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[$i]['id'])" 2>/dev/null)
                bname=$(echo "$buckets_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[$i]['name'])" 2>/dev/null)
                benc=$(echo "$buckets_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[$i].get('encrypted', False))" 2>/dev/null)
                bucket_ids+=("$bid")
                bucket_names+=("$bname")
                local enc_label=""
                if [ "$benc" = "True" ]; then
                    enc_label=" [encrypted]"
                fi
                echo "    $((i + 1)). ${bname}${enc_label}  (${bid})"
            done
            echo ""
        fi
    fi

    # Ask for the source root path — bucket names become subdirectories inside it
    local root_path
    echo "Choose a source root directory for synced files."
    echo "Each selected bucket will become a subdirectory named after the bucket."
    echo "  Example: if root is /home/data and buckets are 'photos' and 'docs',"
    echo "  the service will watch /home/data/photos and /home/data/docs."
    echo ""
    while true; do
        root_path=$(ask_input "Source root directory")
        if validate_path "$root_path"; then
            break
        fi
    done

    # Bucket mappings: folder -> bucketId
    local folders=()
    local mapping_folders=()
    local mapping_bucket_ids=()

    if [ ${#bucket_ids[@]} -gt 0 ]; then
        echo ""
        echo "── Select Buckets to Sync ────────────────────────────"
        echo ""
        echo "Each selected bucket becomes a watched directory under ${root_path}."
        echo "Files in that directory will be uploaded to the corresponding bucket."
        echo ""
        for i in $(seq 0 $((${#bucket_ids[@]} - 1))); do
            if ask_yes_no "  Sync '${bucket_names[$i]}'?" "y"; then
                local dir="${root_path}/${bucket_names[$i]}"
                folders+=("$dir")
                mapping_folders+=("$dir")
                mapping_bucket_ids+=("${bucket_ids[$i]}")
                echo "    ✓ ${dir} → bucket ${bucket_names[$i]} (${bucket_ids[$i]})"
            fi
        done

        if [ ${#folders[@]} -eq 0 ]; then
            echo ""
            echo "  No buckets selected. You must select at least one."
            echo "  Aborted. Re-run configuration with: sudo ${CONFIG_DIR}/configure.sh"
            exit 1
        fi
    else
        # No buckets fetched — ask for manual mappings
        echo ""
        echo "No buckets were loaded. Enter mappings manually."
        echo "Each mapping pairs a watched directory with a bucket ID."
        echo ""
        while true; do
            local folder bucket_id
            echo -n "  Watch folder (empty to finish): " >&2
            read -r folder
            if [ -z "$folder" ]; then
                if [ ${#folders[@]} -eq 0 ]; then
                    echo "  You must add at least one mapping."
                    continue
                fi
                break
            fi
            if validate_path "$folder"; then
                bucket_id=$(ask_input "  Bucket ID for ${folder}")
                folders+=("$folder")
                mapping_folders+=("$folder")
                mapping_bucket_ids+=("$bucket_id")
                echo "    ✓ ${folder} → bucket ${bucket_id}"
            fi
        done
    fi

    # Convert comma-separated scopes to JSON array
    local scopes_json
    scopes_json=$(echo "$scopes" | sed 's/,/","/g')
    scopes_json="[\"${scopes_json}\"]"

    # Build bucketMappings JSON array
    local mappings_json="["
    first=true
    for i in $(seq 0 $((${#mapping_folders[@]} - 1))); do
        if [ "$first" = true ]; then
            first=false
        else
            mappings_json+=","
        fi
        mappings_json+="{\"folder\":\"${mapping_folders[$i]}\",\"bucketId\":\"${mapping_bucket_ids[$i]}\"}"
    done
    mappings_json+="]"

    cat > "$CONFIG_FILE" << EOF
{
    "workDir": "${root_path}",
    "pikaCloud": {
        "baseUrl": "${base_url}",
        "bucketId": "",
        "timeout": ${timeout},
        "retryCount": ${retry_count},
        "oauth2": {
            "issuerUrl": "${issuer_url}",
            "realm": "${realm}",
            "clientId": "${client_id}",
            "scopes": ${scopes_json},
            "tokenFile": "${token_file}"
        }
    },
    "bucketMappings": ${mappings_json}
}
EOF
    chmod 640 "$CONFIG_FILE"

    # Create watched directories
    for f in "${folders[@]}"; do
        mkdir -p "$f" 2>/dev/null || true
    done

    echo ""
    echo "  ✓ Configuration written to ${CONFIG_FILE}"
}

# ─────────────────────────────────────────────────
# Step 3: Summary
# ─────────────────────────────────────────────────
print_summary() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════╗"
    echo "║  Configuration Complete!                              ║"
    echo "╠═══════════════════════════════════════════════════════╣"
    echo "║                                                       ║"
    echo "║  Config file: ${CONFIG_FILE}                          ║"
    echo "║                                                       ║"
    echo "║  Start the service:                                   ║"
    echo "║    sudo systemctl start pikafileservice               ║"
    echo "║                                                       ║"
    echo "║  Check status:                                        ║"
    echo "║    sudo systemctl status pikafileservice              ║"
    echo "║                                                       ║"
    echo "║  View logs:                                           ║"
    echo "║    sudo journalctl -u pikafileservice -f              ║"
    echo "║                                                       ║"
    echo "║  Re-run this wizard:                                  ║"
    echo "║    sudo /opt/pikafileservice/configure.sh             ║"
    echo "╚═══════════════════════════════════════════════════════╝"
    echo ""
}

# ─────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────
main() {
    print_banner

    local mode
    mode=$(choose_mode)

    case "$mode" in
        local)
            configure_local
            ;;
        cloud)
            configure_cloud
            ;;
    esac

    print_summary
}

main "$@"
