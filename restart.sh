kill $(ps aux | grep '[v]arnamd' | awk '{print $2}')
make run >> web.log 2>> errors.log &
