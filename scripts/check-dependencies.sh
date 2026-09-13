#!/bin/sh
set -eu

check_forbidden() {
  package=$1
  shift
  imports=$(go list -f '{{range .Imports}}{{println .}}{{end}}' "$package")
  for forbidden in "$@"; do
    if printf '%s\n' "$imports" | grep -Eq "^github.com/habiohq/atoha/${forbidden}(/|$)"; then
      printf 'forbidden dependency: %s imports %s\n' "$package" "$forbidden" >&2
      exit 1
    fi
  done
}

check_forbidden ./projection execution provider eventlog adapter cmd
check_forbidden ./execution provider eventlog adapter cmd
check_forbidden ./eventlog/... execution provider adapter cmd
check_forbidden ./provider/... execution eventlog adapter cmd
check_forbidden ./adapter/... provider eventlog cmd

printf 'dependency rules: ok\n'
