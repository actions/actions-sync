# Release process

When we want to do a new release, push a git tag with format `v**` and workflow `releases.yml` executes.

This workflow internally uses [go-releaser](https://goreleaser.com/ci/actions/) to push a new release.

Please follow the below detailed steps.
- Create a tag in format `v202205240715`
```code
git tag -a `date "+v%Y%m%d%H%M"` -m "Release a new version"
```
- Get the tag name
```code
git tag
```
- Push the newly created tag
```code
git push origin <tag>
```
- Check that workflow [`releases.yml`](https://github.com/actions/actions-sync/actions/workflows/releases.yml) was triggered
- Once completed, [go to repo releases page](https://github.com/actions/actions-sync/releases) and edit the newly created release as `pre-release`, so we can do sanity testing before we officially release
- Recommend to do basic sanity testing (see below) on the new release.
- Once sanity testing is done, we can edit the release and mark it as `Latest version` and edit the release notes.

## Basic Sanity testing

### Prerequisite

1. Access to a GHES test server
1. Create a PAT token with `site-admin` scope in the GHES environment for `ghe-admin`

### Execution

1. Update below Repository level secrets:

    - sanity_test_site_admin_token: The PAT generated earlier 
    - sanity_test_ghes_url: The URL to the GHES instance 
    - sanity_test_releasedatetime: The tag datetime string for the release to test without the `v` (e.g. `202211070205`)

1. Manually trigger this workflow: https://github.com/actions/actions-sync/actions/workflows/actions-sync-e2e-test-caller.yml
