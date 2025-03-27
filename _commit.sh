#!/bin/sh
git add -A
git -c "user.name=github-actions[bot]" -c "user.email=41898282+github-actions[bot]@users.noreply.github.com" commit -m "Update results" || exit 0
until git pull --rebase && git push; do
    sleep 1
done
