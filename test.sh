#!/bin/sh
set -e
REPO="go-while/GaRuS"
TOKEN="CNF7KJ8RRYZKLAJFPRZ5ZCUA"
GITREF="abc1234"

while true; do
 curl -F "file=@./test.file" -H "X-Git-Repo: $REPO" -H "X-Git-Ref: $GITREF" -H "X-Auth-Token: $TOKEN" http://localhost:58080/upload.php;
 sleep 1
done
