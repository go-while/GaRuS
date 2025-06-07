#!/bin/sh
set -e
REPO="go-while/GaRuS"
TOKEN="CNF7KJ8RRYZKLAJFPRZ5ZCUA"
GITREF="abc1234"
SHA7="1234567"
COMP="SHR"
while true; do
 curl -F "file=@./test.file" -H "X-Git-Repo: $REPO" -H "X-Git-Ref: $GITREF" -H "X-Git-SHA7: $SHA7" -H "X-Git-Comp: $COMP" -H "X-Auth-Token: $TOKEN" http://localhost:58080/upload.php;
 sleep 1
done
