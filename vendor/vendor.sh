#!/bin/bash
set -e

gitVendor() {
	if [ $# -ne 3 ]; then
		echo "Must have three arguments to gitvendor"
		exit 1
	fi
	repo=$1
	dir=./src/$2
	commit=$3
	rm -rf $dir
	mkdir -p $dir
	git clone $repo $dir
	pushd .
	cd $dir
	git checkout $commit
	rm -rf .git
	popd
}

gitVendor "https://code.google.com/p/gomock" "code.google.com/p/gomock" "526771f51633c1315ac61c3d832f536f479e1501"
gitVendor "https://github.com/awslabs/aws-sdk-go.git" "github.com/awslabs/aws-sdk-go" "fa66a1839e64ec5a52a1230821097fb54c6561b7"
gitVendor "https://github.com/vaughan0/go-ini.git" "github.com/vaughan0/go-ini" "a98ad7ee00ec53921f08832bc06ecf7fd600e6a1"
gitVendor "https://github.com/Sirupsen/logrus.git" "github.com/Sirupsen/logrus" "6ba91e24c498b49d0363c723e9e2ab2b5b8fd012"
