#!/bin/bash
#JUST EXAMPLE
#A liitle script for health-check services
#extra checking for docker and nginx
echo "=== Working services ==="
service --status-all 2>/dev/null | grep "+" | awk '{print $4}' | while read s; do
    echo "--- Service: $s ---"
    service $s status 2>&1 | head -5
    echo "--- Last 15 lines from log ---"
    tail -n 15 /var/log/{$s,$s.log,syslog} 2>/dev/null | tail -n 15
done

echo "=== Checking Docker ==="
if which docker >/dev/null 2>&1; then
    docker info 2>&1
    docker system info 2>&1
    docker ps -a 2>&1
    docker images 2>&1
    journalctl -u docker -n 15 2>/dev/null || tail -n 15 /var/log/docker.log 2>/dev/null
else
    echo "Docker is not installed"
fi

echo "=== Checking Nginx ==="
if which nginx >/dev/null 2>&1; then
    nginx -t 2>&1
    nginx -T 2>&1 | grep -i error
    journalctl -u nginx -n 15 2>/dev/null || tail -n 15 /var/log/nginx/error.log 2>/dev/null
else
    echo "Nginx is not installed"
fi
