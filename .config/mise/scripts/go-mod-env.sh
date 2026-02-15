#!/usr/bin/env bash

# allow callers to override explicitly.
if [ -n "${GO_MOD_COMPAT:-}" ]; then
  export GO_MOD_COMPAT
fi

# resolve from go.mod when not provided.
if [ -z "${GO_MOD_COMPAT:-}" ] && [ -f "go.mod" ] && command -v go >/dev/null 2>&1; then
  compat="$(
    go mod edit -json 2>/dev/null \
      | awk -F'"' '/"Go":/ { print $4; exit }' \
      | tr -d '[:blank:]'
  )"

  if [ -n "$compat" ]; then
    export GO_MOD_COMPAT="$compat"
  fi
fi
