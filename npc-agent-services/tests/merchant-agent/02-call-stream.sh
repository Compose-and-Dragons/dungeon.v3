#!/bin/bash
SERVICE_URL=${SERVICE_URL:-"http://0.0.0.0:9102/chat-stream-flow"}

read -r -d '' USER_CONTENT <<- EOM
Hello, my name is Bob Morane. Tell me a story about your adventures.
EOM

read -r -d '' DATA <<- EOM
{
  "data": {
    "message":"${USER_CONTENT}"
  }
}
EOM

# Remove newlines from DATA 
DATA=$(echo ${DATA} | tr -d '\n')

echo "Using DATA: ${DATA}"
echo -e "\n"


callback() {
  echo -ne "$1" 
}

unescape_quotes() {
    local str="$1"
    str="${str//\\\"/\"}"  # Replace \" by "
    echo "$str"
}

remove_quotes() {
    local str="$1"
    str="${str%\"}"   # remove " at the end
    str="${str#\"}"   # remove " at start
    echo "$str"
}

curl --no-buffer --silent ${SERVICE_URL} \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
    -d "${DATA}" \
  | while IFS= read -r line; do
    if [[ $line == data:* ]]; then
      #echo "🤖> ${line#data: }"
      json_data="${line#data: }"
      content_chunk=$(echo "$json_data" | jq '.message // empty' 2>/dev/null)
      if [[ -n "$content_chunk" ]]; then
        result=$(remove_quotes "$content_chunk")
        clean_result=$(unescape_quotes "$result")
        callback "$clean_result"
      fi
    fi
  done

echo ""