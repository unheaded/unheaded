#!/usr/bin/env bash
dir="${1:-build}"
cd "$dir"
make all
