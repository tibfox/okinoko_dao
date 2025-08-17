#!/bin/bash
# generate_mock_test.sh
# Generates mock_test.sh for Go WASM exports (//go:wasmexport)

WASM_FILE="$1"

if [[ -z "$WASM_FILE" ]]; then
    echo "Usage: $0 <path-to-wasm-file>"
    exit 1
fi

if [[ ! -f "$WASM_FILE" ]]; then
    echo "WASM file not found: $WASM_FILE"
    exit 1
fi

# Extract Go-exported functions from wasm2wat
EXPORTS=$(wasm2wat "$WASM_FILE" | grep 'func \$' | awk '{print $2}' | tr -d '$')

if [[ -z "$EXPORTS" ]]; then
    echo "No Go-exported functions found in $WASM_FILE"
    exit 1
fi

# Load mock args config if present
CONFIG_FILE="mock_args.conf"
declare -A FUNC_ARGS
if [[ -f "$CONFIG_FILE" ]]; then
    while IFS= read -r line; do
        [[ -z "$line" || "$line" =~ ^# ]] && continue

        name="${line%%=*}"
        rhs="${line#*=}"

        args=$(echo "$rhs" | awk '
            {
                s=$0
                while (length(s)) {
                    sub(/^[ \t]+/, "", s)  # trim leading spaces

                    # JSON object
                    if (match(s, /^\{[^}]*\}/)) {
                        printf "\047%s\047 ", substr(s, RSTART, RLENGTH)  # single quotes
                        s = substr(s, RSTART+RLENGTH)
                        continue
                    }

                    # JSON array
                    if (match(s, /^\[[^]]*\]/)) {
                        printf "\047%s\047 ", substr(s, RSTART, RLENGTH)  # single quotes
                        s = substr(s, RSTART+RLENGTH)
                        continue
                    }

                    # Number
                    if (match(s, /^[0-9]+(\.[0-9]+)?/)) {
                        printf "%s ", substr(s, RSTART, RLENGTH)  # unquoted
                        s = substr(s, RSTART+RLENGTH)
                        continue
                    }

                    # Otherwise: treat as string
                    if (match(s, /^[^ \t]+/)) {
                        printf "\"%s\" ", substr(s, RSTART, RLENGTH)
                        s = substr(s, RSTART+RLENGTH)
                        continue
                    }

                    break
                }
            }')

        FUNC_ARGS["$name"]="$args"
    done < "$CONFIG_FILE"
fi





# Create mock_test.sh
OUTPUT_SCRIPT="mock_test.sh"
echo "#!/bin/bash" > "$OUTPUT_SCRIPT"
echo "# Auto-generated mock test for $WASM_FILE (Go WASM exports)" >> "$OUTPUT_SCRIPT"
echo "" >> "$OUTPUT_SCRIPT"

COUNT=0

for func in $EXPORTS; do
    if [[ "$func" == *projects_* ]] || [[ "$func" == *proposals_* ]]; then
    
        echo "Running $func..."

        echo "echo \"Running $func...\"" >> "$OUTPUT_SCRIPT"

        if [[ -n "${FUNC_ARGS[$func]}" ]]; then
            echo "wasmtime $WASM_FILE --invoke $func ${FUNC_ARGS[$func]}" >> "$OUTPUT_SCRIPT"
        else
            echo "wasmtime $WASM_FILE --invoke $func" >> "$OUTPUT_SCRIPT"
        fi

        echo "" >> "$OUTPUT_SCRIPT"
        COUNT=$((COUNT + 1))
    fi
done

chmod +x "$OUTPUT_SCRIPT"
echo "Generated $OUTPUT_SCRIPT with $COUNT exported functions."


# #!/bin/bash
# # generate_mock_test.sh
# # Generates mock_test.sh for Go WASM exports (//go:wasmexport)

# WASM_FILE="$1"

# if [[ -z "$WASM_FILE" ]]; then
#     echo "Usage: $0 <path-to-wasm-file>"
#     exit 1
# fi

# if [[ ! -f "$WASM_FILE" ]]; then
#     echo "WASM file not found: $WASM_FILE"
#     exit 1
# fi

# # Extract Go-exported functions from wasm2wat
# EXPORTS=$(wasm2wat "$WASM_FILE" | grep 'func \$' | awk '{print $2}' | tr -d '$')

# if [[ -z "$EXPORTS" ]]; then
#     echo "No Go-exported functions found in $WASM_FILE"
#     exit 1
# fi

# # Create mock_test.sh
# OUTPUT_SCRIPT="mock_test.sh"
# echo "#!/bin/bash" > "$OUTPUT_SCRIPT"
# echo "# Auto-generated mock test for $WASM_FILE (Go WASM exports)" >> "$OUTPUT_SCRIPT"
# echo "" >> "$OUTPUT_SCRIPT"

# COUNT=0

# for func in $EXPORTS; do
#     # Add a dummy invocation line (empty args; adjust if needed)
#      if [[ "$func" == *projects_* ]] || [[ "$func" == *proposals_* ]];  then
#         echo "echo \"Running $func...\"" >> "$OUTPUT_SCRIPT"
#         echo "wasmtime run --invoke $func $WASM_FILE" >> "$OUTPUT_SCRIPT"
#         echo "" >> "$OUTPUT_SCRIPT"
#         COUNT=$((COUNT + 1))
#     fi
# done

# chmod +x "$OUTPUT_SCRIPT"
# echo "Generated $OUTPUT_SCRIPT with $COUNT exported functions."
