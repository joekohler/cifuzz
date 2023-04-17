#!/bin/bash
set -e

check_lines() {
	local txt_folder=$1
	while read -r line; do
			found=false
			for file in "$txt_folder"*.txt; do
					if grep -Fxq "$line" "$file"; then
							found=true
							break
					fi
			done
			if ! $found; then
					echo "$line" >> "$txt_folder""$(ls -1 $txt_folder | head -1)"
			fi
	done <<< "$packages"
}

packages=$(go list ./...)
check_lines "./tools/test-bucket-generator/win/" <<< "$packages"
check_lines "./tools/test-bucket-generator/linux-mac/" <<< "$packages"
