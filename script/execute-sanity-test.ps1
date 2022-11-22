# Testing Pull Single Repo
echo "`n#########################`n### Testing Pull Single Repo`n#########################"
bin/actions-sync pull --cache-dir "cache"  --repo-name "actions/setup-node"

# Testing Sync Single Repo
echo "`n#########################`n### Testing Sync Single Repo`n#########################"
bin/actions-sync sync --cache-dir "cache" --destination-token $Env:TOKEN --destination-url $Env:TEST_INSTANCE_URL --repo-name "actions/setup-node" --actions-admin-user actions-admin

# Testing Pull Multiple Repos
echo "`n#########################`n### Testing Pull Multiple Repos`n#########################"
bin/actions-sync pull --cache-dir "cache"  --repo-name-list "actions/setup-node,actions/checkout"

# Testing Push Multiple Existing Repos
echo "`n#########################`n### Testing Push Multiple Existing Repos`n#########################"
bin/actions-sync push --cache-dir "cache" --destination-token $Env:TOKEN --destination-url $Env:TEST_INSTANCE_URL --repo-name-list "actions/setup-node,actions/checkout" --actions-admin-user actions-admin

# Testing Sync Multiple Existing Repos
echo "`n#########################`n### Testing Sync Multiple Existing Repos`n#########################"
bin/actions-sync sync --cache-dir "cache" --destination-token $Env:TOKEN --destination-url $Env:TEST_INSTANCE_URL --repo-name-list "actions/setup-node,actions/checkout" --actions-admin-user actions-admin

# Testing Sync New Single Repo
echo "`n#########################`n### Testing Sync New Single Repo`n#########################"
bin/actions-sync sync --cache-dir "cache" --destination-token $Env:TOKEN --destination-url $Env:TEST_INSTANCE_URL --repo-name-list "actions/actions-sync" --actions-admin-user actions-admin
