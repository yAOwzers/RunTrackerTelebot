#!/bin/zsh

docker compose down 

docker build -t runtrackerbot -f Dockerfile_backup . && echo "Building..."                                                                                 ─╯

docker compose down 

docker compose up -d

docker logs -f runTrackerBot
