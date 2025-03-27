#!/bin/sh
git add -A
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git commit -m "Update results" || exit 0
until git pull --rebase && git push; do
    sleep 1
done
