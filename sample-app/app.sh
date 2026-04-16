#!/bin/sh
echo "------------------------------------------------"
echo "Docksmith Sample Application Starting..."
echo "Current User: $(whoami)"
echo "Working Directory: $(pwd)"
echo "Environment Variable APP_MSG: ${APP_MSG:-'Not Set'}"
echo "Environment Variable KEY: ${KEY:-'Not Set'}"
echo "------------------------------------------------"
echo "Contents of /app:"
ls -F /app
echo "------------------------------------------------"
if [ -f "/app/data.txt" ]; then
    echo "Data file content:"
    cat /app/data.txt
fi
echo "------------------------------------------------"
echo "Docksmith Sample Application Finished."
