#!/bin/bash

set -e

# Decrypt GCR deploy key
openssl aes-256-cbc -K $encrypted_343d449a08cc_key -iv $encrypted_343d449a08cc_iv -in gcr-deploy.json.enc -out gcr-deploy.json -d

# Make sure we've got a gcloud client ...
if [ ! -d "$HOME/google-cloud-sdk/bin" ]
then
  rm -rf $HOME/google-cloud-sdk
  curl https://sdk.cloud.google.com | bash
fi

# Add gcloud to our PATH
source ~/google-cloud-sdk/path.bash.inc

# Activate travis-container-builder service account
gcloud auth activate-service-account travis-container-builder@freenome-build.iam.gserviceaccount.com --key-file gcr-deploy.json
make docker-prod
gcloud docker -- push gcr.io/freenome-build/${SERVICE_NAME}:${TRAVIS_COMMIT}
