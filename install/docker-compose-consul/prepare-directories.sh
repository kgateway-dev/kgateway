#!/usr/bin/env bash

echo "Cleaning the 'data' folder"
rm -rf ./data

echo "Copy the source data into the 'data' folder"
cp -r source_data data

mkdir -p ./data/artifacts/gloo-system

echo "Done"
