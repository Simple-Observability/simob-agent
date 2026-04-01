scc \
  --by-file \
  -x json \
  -x jsonl \
  -f wide \
  -s code \
  --no-cocomo \
  --no-size \
  --not-match "_test\.go$" \
  --exclude-dir thirdparty \
  agent/
