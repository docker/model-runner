# Contributing

Pull requests are welcomed. You must

- Sign the Elastic [Contributor License Agreement](https://www.elastic.co/contributor-agreement).
- Include a [changelog][changelog_docs] entry at `.changelog/{pr-number}.txt` with your pull request.
- Include tests that demonstrate the change is working.

[changelog_docs]: https://github.com/GoogleCloudPlatform/magic-modules/blob/2834761fec3acbf35cacbffe100530f82eada650/.ci/RELEASE_NOTES_GUIDE.md#expected-format

## Releasing

To create a new release use the release workflow in GitHub actions. This will create a new draft
release in GitHub releases with a changelog. After the job completes, review the draft and if
everything is correct, publish the release. When the release is published GitHub will create the
git tag.
