# Skills (agent instructions)

Each subdirectory is a skill the generation agent loads for a product type:

| Skill | Status | What it is |
|---|---|---|
| `pptx` | published | presentation skill (layouts, charts, LibreOffice helpers) |
| `satbayev-gost` | **not published** (gitignored) | document skill for the Satbayev University standard |

## Why `satbayev-gost` is not in the repository

It carries the university's official title-page template (`assets/titulki-template.docx`),
the university logo and the formatting rules of the organisation standard. Those
materials are not ours to publish, so the directory is excluded from git
(`.gitignore`: `backend/skills/satbayev-gost/`).

The files may still exist in your working copy - that is intentional: `deploy/push.ps1`
packs the working tree, so the server keeps receiving the skill and the documents
keep coming out in the right format. It is simply not part of the public repository.

## How to install it

1. Put your copy into `backend/skills/satbayev-gost/` (the directory name matters -
   it is the skill name the worker looks for, see `GENERATOR_SKILL_GOST` in
   `backend/.env`).
2. Deploy as usual: `.\push.ps1 -Server root@<host> -Domain <domain> ...`.
3. Check that the agent picked it up: the generation log line says
   `скилл satbayev-gost`; if the directory is missing the worker falls back to the
   built-in instructions (`backend/internal/worker/skill/document.md`) and the
   documents lose the official title page.

## Structure a skill needs

```
<skill-name>/
├── SKILL.md          # the instructions the agent reads (copied to .clinerules/01-skill.md)
├── references/       # detailed rules the agent may open
├── assets/           # templates, logos, checklists
└── scripts/          # helper scripts the agent may run
```
