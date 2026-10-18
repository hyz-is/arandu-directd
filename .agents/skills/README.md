# Skills

Procedures an assistant follows when working on this package.

They live in `.agents/skills/<name>/SKILL.md`, which is the path the coding
assistants read from. It is one directory rather than a file per vendor, so a
skill written once is read by whatever this package is being written with.

Each file opens with frontmatter carrying a `name` and a `description`. The
`name` has to equal the directory name exactly, or the skill is not loaded. The
description is what a tool reads to decide whether the skill is relevant, so it
names the situation you are in rather than the topic it covers.

| skill | when it fires |
| --- | --- |
| `directd-module` | changing the client, the meter, the outcomes, the async flow, the catalogue, the generator or a command |
| `directd-release` | the gates, the manifest, a dependency, a version, a tag |
| `directd-package` | installing and wiring this package **into an application** |

The last one has a different audience, and that is on purpose: it travels with
the package (`metadata: audience: app`, which `aru skills:sync` reads) so that
an assistant working in somebody else's project has the wiring, the Meter and
the cost rules in front of it instead of guessing.

## Why these exist

The common failure modes are a test that "quickly checks" the real API (every
query is billed), a guessed status code or callback scheme written down as
documented, a verdict field on the result, a query made without a Ledger hold,
a tenant read from the request, a token printed in a log or an error, and a
hand edit to a generated file. None belongs here.
