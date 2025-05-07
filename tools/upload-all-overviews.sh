#!/bin/bash

# Check if username and token are provided
if [ -z "$1" ] || [ -z "$2" ]; then
  echo "Usage: $0 <username> <token>"
  exit 1
fi

USERNAME=$1
TOKEN=$2
AI_DIR="./ai"
CLI_PATH="./tools/model-cards-cli/bin/model-cards-cli"

# Ensure the CLI tool is built
(cd tools/model-cards-cli && make build)

# Find all markdown files and count them
files=("$AI_DIR"/*.md)
file_count=${#files[@]}

echo "Found $file_count markdown files in the ai directory."
echo "The following files will be uploaded to their corresponding repositories:"

# List all files that will be uploaded
for file in "${files[@]}"; do
  filename=$(basename "$file" .md)
  repo="ai/$filename"
  echo "  - $file -> $repo"
done

# Ask for confirmation
echo ""
read -p "Do you want to proceed with uploading these overviews? (y/n): " confirm
if [[ $confirm != [yY] && $confirm != [yY][eE][sS] ]]; then
  echo "Upload cancelled."
  exit 0
fi

echo "Starting uploads..."
echo ""

# Process each file
for file in "${files[@]}"; do
  # Extract the filename without path and extension
  filename=$(basename "$file" .md)
  
  # Construct the repository name
  repo="ai/$filename"
  
  echo "Uploading overview from $file to $repo..."
  
  # Call the upload-overview command
  $CLI_PATH upload-overview --file="$file" --repository="$repo" --username="$USERNAME" --token="$TOKEN"
  
  # Check if the command was successful
  if [ $? -eq 0 ]; then
    echo "✅ Successfully uploaded $file to $repo"
  else
    echo "❌ Failed to upload $file to $repo"
  fi
  
  echo ""
  
  # Add a small delay between uploads to avoid rate limiting
  sleep 1
done

echo "Upload process completed!"
