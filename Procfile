# The Agency — Overmind process file
# Boot: overmind start
# Stop: overmind stop
# Logs: overmind connect <process>
# Prerequisites: scripts/build-daemons, redis-server in PATH

redis:     redis-server --save "" --appendonly no --loglevel notice
office:    ./dist/agency-office-daemon
runtime:   ./dist/agency-runtime-daemon
scheduler: ./dist/agency-scheduler-daemon
