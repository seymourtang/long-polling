#!/bin/bash

ADDR=$1
KEY=$2
VERSION=
reponse=''
lastUpdated=
while true
do
	response=`curl -s http://${ADDR}/updates\?key=${KEY}\&\&version=${VERSION}\&\&watch=60`
	lastUpdated=`echo $response | jq '.[0].lastUpdated'`
	if [ -n "$lastUpdated" ]
	then
	echo $response
	VERSION=$lastUpdated
	fi

done
