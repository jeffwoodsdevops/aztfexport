#!/bin/bash
set -e

version=${1:?"version not specified"}

rpm --import https://packages.microsoft.com/keys/microsoft.asc

# To accelarate syncing the repo metadatas
rm -f /etc/yum.repos.d/*

dnf install -y https://packages.microsoft.com/config/fedora/34/packages-microsoft-prod.rpm

# See: https://access.redhat.com/solutions/2779441
dnf check-update || [[ $? == 100 ]]  

dnf install -y aztfy
grep $version <(aztfy -v)
