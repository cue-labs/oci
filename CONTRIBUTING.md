
# Contribution Guide


## Before contributing code

As with many open source projects, we use the GitHub [issue
tracker](https://github.com/cue-labs/oci/issues) to not only track bugs, but
also coordinate work on new features, bugs, designs and proposals.  Given the
inherently distributed nature of open-source this coordination is important
because it very often serves as the main form of communication between
contributors.

You can also exchange ideas or feedback with other contributors via the CUE
project's `#contributing` [Slack
channel](https://cuelang.slack.com/archives/CMY132JKY), as well as the
contributor office hours calls which we hold via the [community
calendar](https://cuelang.org/s/community-calendar) once per week.

### Check the issue tracker

Whether you already know what contribution to make, or you are searching for an
idea, the [issue tracker](https://github.com/cue-labs/oci/issues) is always the
first place to go.  Issues are triaged to categorize them and manage the
workflow.

TODO: add detail on how we label and categorise issues.

### Open an issue for any new problem

Excluding very trivial changes, all contributions should be connected to an
existing issue.  Feel free to open one and discuss your plans.  This process
gives everyone a chance to validate the design, helps prevent duplication of
effort, and ensures that the idea fits inside the goals for the language and
tools.  It also checks that the design is sound before code is written; the code
review tool is not the place for high-level discussions.

Sensitive security-related issues should be reported to <a
href="mailto:security@cue.works">security@cue.works</a>.

## Becoming a code contributor

The code contribution process used by the this project, like the [CUE
project](https://cuelang.org), is a little different from that used by other
open source projects.  We assume you have a basic understanding of
[`git`](https://git-scm.com/) and [Go](https://golang.org) (1.21 or later).

The first thing to decide is whether you want to contribute a code change via
GitHub or GerritHub. Both workflows are fully supported, and whilst GerritHub is
used by the core project maintainers as the "source of truth", the GitHub Pull
Request workflow is 100% supported - contributors should feel entirely
comfortable contributing this way if they prefer.

Contributions via either workflow must be accompanied by a Developer Certificate
of Origin.

### Asserting a Developer Certificate of Origin

Contributions must be accompanied by a [Developer Certificate of
Origin](https://developercertificate.org/), the text of which is reproduced here
for convenience:

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
1 Letterman Drive
Suite D4700
San Francisco, CA, 94129

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

All commit messages must contain the `Signed-off-by` line with an email address
that matches the commit author. This line asserts the Developer Certificate of Origin.

When committing, use the `--signoff` (or `-s`) flag:

```console
$ git commit -s
```

You can also [set up a prepare-commit-msg git
hook](#do-i-really-have-to-add-the--s-flag-to-each-commit) to not have to supply
the `-s` flag.

The explanations of the GitHub and GerritHub contribution workflows that follow
assume all commits you create are signed-off in this way.


## Preparing for GitHub Pull Request (PR) Contributions

TODO: add detail.

## Preparing for GerritHub [CL](https://google.github.io/eng-practices/#terminology) Contributions

TODO: add detail.

## Good commit messages

Commit messages follow a specific set of conventions, which we discuss in this
section.

Here is an example of a good one taken from the [CUE
project](https://cuelang.org):


```
cue/ast/astutil: fix resolution bugs

This fixes several bugs and documentation bugs in
identifier resolution.

1. Resolution in comprehensions would resolve identifiers
to themselves.

2. Label aliases now no longer bind to references outside
the scope of the field. The compiler would catch this invalid
bind and report an error, but it is better not to bind in the
first place.

3. Remove some more mentions of Template labels.

4. Documentation for comprehensions was incorrect
(Scope and Node were reversed).

5. Aliases X in `X=[string]: foo` should only be visible
in foo.

Fixes #946
```

### First line

The first line of the change description is conventionally a short one-line
summary of the change, prefixed by the primary affected package
(`cue/ast/astutil` in the example above).

A rule of thumb is that it should be written so to complete the sentence "This
change modifies CUE to \_\_\_\_." That means it does not start with a capital
letter, is not a complete sentence, and actually summarizes the result of the
change.

Follow the first line by a blank line.


### Main content

The rest of the description elaborates and should provide context for the change
and explain what it does.  Write in complete sentences with correct punctuation,
just like for your comments. Don't use HTML, Markdown, or any other markup
language.


### Referencing issues

The special notation `Fixes #12345` associates the change with issue 12345 in
the [issue tracker](https://github.com/cue-labs/oci/issue/12345) When this
change is eventually applied, the issue tracker will automatically mark the
issue as fixed.


If the change is a partial step towards the resolution of the issue, uses the
notation `Updates #12345`.  This will leave a comment in the issue linking back
to the change in Gerrit, but it will not close the issue when the change is
applied.


All issues are tracked in the main repository's issue tracker.
If you are sending a change against a subrepository, you must use the
fully-qualified syntax supported by GitHub to make sure the change is linked to
the issue in the main repository, not the subrepository (eg. `Fixes
cue-lang/cue#999`).


## The review process

TODO: add detail.

## Miscellaneous topics

This section collects a number of other comments that are outside the
issue/edit/code review/submit process itself.



### Copyright headers

TODO: add detail.

### Do I really have to add the `-s` flag to each commit?

Earlier in this guide we explained the role the [Developer Certificate of
Origin](https://developercertificate.org/) plays in contributions to the CUE
project. we also explained how `git commit -s` can be used to sign-off each
commit. But:

* it's easy to forget the `-s` flag;
* it's not always possible/easy to fix up other tools that wrap the `git commit`
  step.

You can automate the sign-off step using a [`git`
hook](https://git-scm.com/book/en/v2/Customizing-Git-Git-Hooks). Run the
following commands in the root of a `git` repository where you want to
automatically sign-off each commit:

```
cat <<'EOD' > .git/hooks/prepare-commit-msg
#!/bin/sh

NAME=$(git config user.name)
EMAIL=$(git config user.email)

if [ -z "$NAME" ]; then
    echo "empty git config user.name"
    exit 1
fi

if [ -z "$EMAIL" ]; then
    echo "empty git config user.email"
    exit 1
fi

git interpret-trailers --if-exists doNothing --trailer \
    "Signed-off-by: $NAME <$EMAIL>" \
    --in-place "$1"
EOD
chmod +x .git/hooks/prepare-commit-msg
```

If you already have a `prepare-commit-msg` hook, adapt it accordingly. The `-s`
flag will now be implied every time a commit is created.



## Code of Conduct

We follow guidelines for participating in CUE community spaces and a reporting
process for handling issues can be found in the [Code of
Conduct](https://cuelang.org/docs/contribution_guidelines/conduct).
