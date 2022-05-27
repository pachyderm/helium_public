#!/usr/bin/env bash
if [ $BRANCH_NAME == "main" ] ; then
  mkdir -p ./bin
  wget -O ./bin/version-bump https://github.com/pachyderm/version-bump/releases/download/v0.0.3/version-bump-amd64-v0.0.3
  chmod a+x ./bin/version-bump
  ./bin/version-bump --owner=pachyderm --repo=cluster-config --file=argocd/workloads/production/helium/deployment.yaml --branch=master --location=spec.template.spec.containers.[name=helium].image --replacement=$SHORT_SHA --author-name=***REMOVED*** --author-email=buildbot@***REMOVED*** --token=$***REMOVED*** --message="bump helium version"
  ./bin/version-bump --owner=pachyderm --repo=cluster-config --file=argocd/workloads/production/helium/cp-deployment.yaml --branch=master --location=spec.template.spec.containers.[name=helium-cp].image --replacement=$SHORT_SHA --author-name=***REMOVED*** --author-email=buildbot@***REMOVED*** --token=$***REMOVED*** --message="bump helium-cp version"
fi
