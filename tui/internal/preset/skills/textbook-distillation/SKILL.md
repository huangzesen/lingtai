---
name: textbook-distillation
description: >
  Turn a textbook or long-form source into a self-paced learning track:
  chapter map, lesson plan, then self-contained HTML lecture notes (styled
  as the human specifies) with worked examples, exercises, and checkpoint
  questions. Read this for "lecture notes" / "study notes" / "course" /
  "syllabus" requests from a textbook or PDF. Do NOT use to reproduce or
  redistribute a copyrighted book verbatim, to "summarize so I don't have
  to buy it," or for one-off factual lookups (just answer those directly).
version: 1.0.0
tags: [learning, study, textbook, lecture-notes, html, curriculum, self-paced]
last_changed_at: "2026-09-05T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# Textbook Distillation — Self-Paced Learning Tracks

You are helping a human teach themselves from a textbook (or a long technical
document, lecture transcript, or paper set) without a live instructor. Your job
is to **distill** the source into a structured learning track and ship
**self-contained HTML lecture notes** in the style the human asks for: extract
the concepts, structure, and worked logic, then re-explain them in your own
words with your own examples. Distillation is never reproduction of the author's
text. Read [Source limits & safety](#source-limits--copyright--safety) before you
intake anything — those boundaries shape every later step.

## When to use

The frontmatter `description` carries the triggers and anti-triggers. Two
boundaries deserve restating here: an ask framed as "summarize the whole book so
I can skip buying it" or "paste me the chapter" crosses the copyright line —
decline it and offer a legitimate distillation instead; and material the
human cannot point to a legitimate source for is out of scope entirely. Both are
spelled out under "Source limits, copyright & safety" below.

## Workflow at a glance

```
intake → chapter map → lesson plan → (per lesson) HTML lecture notes → review loop
```

Work the phases in order. Do not jump to generating HTML before the human has
seen and approved the chapter map and lesson plan — that approval is what keeps
the track aligned to *their* goal, not your guess at it. This is a multi-round,
human-in-the-loop flow: **talk to the human through the active user-facing channel
(mail/email or the relevant chat bridge), not internal scratch/text output**,
because each phase needs their input.

## Phase 1 — Intake the source

Goal: know exactly what you are distilling and confirm you are allowed to.

1. **Identify the source precisely.** Title, author, edition, and the format you
   have access to (a PDF in the project, a URL, the human's own notes, a file the
   human pasted). Record the path or link — you will cite it in every artifact.
2. **Confirm authorization and scope.** Ask the human: do they own/have legal
   access to this material? Which parts do they want covered — whole book, a
   range of chapters, one topic? Do not proceed on material they cannot point to
   a legitimate source for.
3. **Capture the learning goal.** Why are they studying it — exam, project,
   curiosity, teaching it onward? Their level (beginner / refresher / advanced)?
   Time budget (one weekend vs. a semester)? These set lesson granularity.
4. **Capture style constraints early** (see [Style constraints](#style-constraints-from-the-human)).
   Collect them now so the lesson plan and the HTML are designed for them, not
   retrofitted.

If you only have a partial source (e.g. a table of contents but not the body),
say so explicitly — you will distill structure from the ToC and re-teach concepts
from your own knowledge, clearly labeled, rather than inventing the book's
specific treatment.

## Phase 2 — Build the chapter map

A **chapter map** is the skeleton: the source's structure plus what each unit is
*about* and what it depends on. It is the contract the human approves before any
lesson is written.

For each chapter / major section, capture:

- **Unit** — chapter or section title and number.
- **Core concepts** — the 3–8 ideas a learner must walk away with.
- **Prerequisites** — which earlier units (or outside knowledge) it assumes.
- **Difficulty / weight** — rough effort, so the track can be paced.
- **Source location** — page range or section anchor, for citation.

Render the map as a table and send it to the human. Ask: *Is this the right
scope and ordering? Anything to drop, merge, or go deeper on?* Iterate until they
approve. The map may reorder or merge the book's units to fit the learning goal —
note where you deviate from the book's own order and why.

## Phase 3 — Draft the lesson plan

Turn the approved chapter map into a sequence of **lessons**. A lesson is one
sitting's worth of learning — usually one chapter, or a slice of a dense one.

For each lesson define:

- **Title and one-line objective** ("After this lesson you can …").
- **Concepts covered** — drawn from the chapter map, scoped to one sitting.
- **Prerequisite lessons** — what must come first.
- **Worked example(s)** — the concrete problem you will walk through.
- **Exercises** — 2–5 practice problems, easy → hard.
- **Checkpoint** — 3–6 self-check questions (with answers, collapsible) that
  tell the learner whether they can move on.
- **Estimated time.**

Send the lesson plan for approval. This is the last gate before you generate
HTML, so make sure granularity, ordering, and exercise depth match what the
human wanted.

## Phase 4 — Generate the HTML lecture notes

Now produce the deliverable: **one self-contained HTML file per lesson** (or a
single file for a short track), in the human's style.

**Standalone-HTML mechanics are owned by the `html-report` reference** — the
skeleton, artifact hygiene, the validation checklist, and, critically, **MathJax
setup**, since lecture notes for technical books almost always contain equations.
Read `~/.lingtai-tui/utilities/swiss-knife/reference/html-report/SKILL.md` (or
load `swiss-knife` and route to `html-report`) and follow it rather than
re-deriving it here.

Start from the bundled template at
`~/.lingtai-tui/utilities/textbook-distillation/assets/lesson-template.html`,
which already wires MathJax, a print stylesheet, collapsible checkpoint answers,
callout boxes, CSS variables for theming, and the lesson structure below. Copy
it, apply the human's style, fill in the content.

Each lesson must carry, in order:

1. **Header** — title, objective, source citation (title, author, edition, page
   range), generation timestamp (`date -u +%Y-%m-%dT%H:%M:%SZ`), and the
   provenance caveat ("Distilled study notes — re-explained from the source, not
   a reproduction of it.").
2. **Prerequisites** — a short "before this lesson" note linking prior lessons.
3. **Concept exposition** — the teaching, in *your own words*, terms defined in
   callout boxes, with diagrams / tables / analogies as the style calls for.
4. **Worked example(s)** — step through the reasoning explicitly.
5. **Exercises** — numbered, increasing difficulty, solutions hidden behind a
   `<details>` element so the learner attempts first.
6. **Checkpoint** — self-check questions with collapsible answers; passing them
   is the signal to advance.
7. **Footer** — "next lesson" link and the source citation again.

Run the `html-report` validation checklist against every file you write. Write
them to a project-local path the human can find (e.g.
`study/<book-slug>/lesson-NN.html`), never `/tmp`.

## Phase 5 — Review loop

Send the first lesson and ask the human to react to **both** the teaching and the
style before you batch the rest — it is far cheaper to adjust the template once
than to regenerate twenty files. Then proceed lesson by lesson (or in small
batches), checking in. Offer an index page linking all lessons once the track is
built.

## Style constraints from the human

Capture these in Phase 1 and apply them in Phase 4. If the human is vague,
propose a default and confirm:

- **Theme / palette** — light or dark, accent color, any brand colors. The
  template exposes these as CSS variables at the top of `<style>`.
- **Layout** — single column vs. sidebar nav; density (airy vs. compact).
- **Emphasis pattern** — what gets boxed/highlighted (definitions, theorems,
  warnings, key formulas). Honor the human's emphasis hierarchy consistently.
- **Tone** — formal textbook, friendly tutor, terse cheat-sheet.
- **Visual aids** — how much diagramming, whether to use tables for comparisons,
  whether to include figures (describe-and-draw with HTML/SVG, do not copy the
  book's figures — see safety).
- **Length per lesson** — full exposition vs. condensed review.

When the human gives a style, *reflect it back* in one line before generating, so
a mismatch is caught before twenty files are produced.

## Source limits, copyright & safety

These boundaries are not optional. They apply from intake onward.

- **Distill, do not reproduce.** Re-explain concepts in your own words with your
  own examples. Do not copy the author's prose, do not paste chapters or long
  passages, and do not recreate the book's figures/tables verbatim. Short quotes
  for commentary are fine; wholesale reproduction is not.
- **No piracy enabler.** Decline requests framed as "summarize the whole book so
  I don't have to buy/read it" or "give me the full text." Offer legitimate
  distillation (concepts, your own worked examples, study aids) instead.
- **Always cite the source.** Every artifact names title, author, edition, and
  the page/section range it draws from, plus a provenance caveat that these are
  re-explained study notes, not the original.
- **Confirm authorized access.** Only work from material the human can point to a
  legitimate source for (owned copy, library/institutional access, open-access,
  their own notes). If they cannot, stop and say so.
- **Flag the limits of your knowledge.** When you teach from your own knowledge
  rather than the specific source (e.g. you only have the ToC), label it clearly
  so the learner knows what is the book's treatment vs. general background. Note
  that your knowledge has a cutoff and the edition may differ — tell the human to
  verify specifics against the actual source.
- **Stay in the learner's interest.** No fabricated citations, no invented page
  numbers, no confidently wrong technical claims. If unsure of a fact, say so and
  point the learner to the source section to confirm.

## Quick reference

| Phase | Output | Gate before next phase |
|---|---|---|
| 1 Intake | source identified, goal + style captured, access confirmed | — |
| 2 Chapter map | table: units, concepts, prereqs, weight, source location | human approves scope/order |
| 3 Lesson plan | sequenced lessons w/ objective, examples, exercises, checkpoint | human approves granularity |
| 4 HTML notes | one self-contained styled `.html` per lesson | passes html-report checklist |
| 5 Review loop | first lesson reviewed, then batch + index page | human reacts to lesson 1 |

## Common mistakes

| Mistake | Fix |
|---|---|
| Pasting/paraphrasing the book closely | Re-explain in your own words with your own examples |
| Skipping exercises/checkpoints | A learning track without practice is just a summary |
| Ignoring stated style, then regenerating all files | Reflect style back, review lesson 1 before batching |
| Inventing the book's specific treatment from memory | Label own-knowledge sections; cite the source for specifics |

---
> **Found a bug or issue?** If you encounter any problems with this skill, load the `lingtai-issue-report` skill and follow its instructions to report it.
