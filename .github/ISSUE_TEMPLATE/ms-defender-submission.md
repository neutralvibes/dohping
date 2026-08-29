---
title: "Defender false-positive submission: v{{ version }} windows binary"
labels: [ms-defender-submission]
---

This tracks clearing a false-positive detection of the Windows binary
with Microsoft Defender. A common issue for statically-linked Go
binaries, which Defender's ML can flag despite being clean.

## Release metadata

| Field | Value |
|---|---|
| Release | v{{ version }} |
| Commit | `{{ commit }}` |
| Artifact | `{{ artifact }}` |
| Archive | `{{ archive }}` |
| SHA-256 | `{{ sha256 }}` |

## Submission (paste into the portal)

> Static Go binary, false-positive detection `{{ detection }}`. Please review.

## Checklist

- [ ] Submit Windows artifact to Microsoft
- [ ] Record submission ID
- [ ] Record result
- [ ] Close issue
