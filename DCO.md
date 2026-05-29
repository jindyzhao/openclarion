# Developer Certificate of Origin

This project uses the Developer Certificate of Origin as a lightweight mechanism
for contributors to certify that they have the right to submit their work.

## Sign-Off

Every commit must include:

```text
Signed-off-by: Your Name <your.email@example.com>
```

The sign-off email must match the commit author email. GitHub
`users.noreply.github.com` author addresses are rejected because they weaken
contributor accountability.

Use:

```bash
git commit -s
```

If you forgot to sign off:

```bash
git commit --amend -s
```

For multiple commits:

```bash
git rebase --signoff HEAD~N
```

## Certificate

By contributing, you certify that your contribution is compatible with the
project license and that you have the right to submit it under that license.

For the full text, see <https://developercertificate.org/>.
