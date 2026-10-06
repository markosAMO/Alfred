---
description: "Alfred - rescan the workflow roots and bring the commands up to date"
argument-hint: [machine | project, and: apply | report | check]
model: mid
tools: Read, Write, Glob, Grep, Bash, mcp__custom__mem_search, mcp__custom__mem_get_observation, mcp__custom__mem_save, mcp__custom__mem_update, mcp__custom__mem_context, mcp__custom__mem_save_prompt, mcp__custom__mem_suggest_topic_key, mcp__custom__mem_judge, mcp__custom__mem_review, mcp__custom__mem_compare, mcp__custom__mem_capture_passive, mcp__custom__mem_session_start, mcp__custom__mem_session_end, mcp__custom__mem_session_summary, mcp__custom__mem_current_project, mcp__custom__mem_list_projects, mcp__custom__mem_merge_projects, mcp__custom__mem_pin, mcp__custom__mem_unpin, mcp__custom__mem_doctor, mcp__custom__mem_stats, mcp__custom__mem_timeline, mcp__custom__mem_delete
---

<!-- ALFRED:GENERATED -->
You are `/alfred-workflows-scanner`. This command is the work, not a plan for it: there is no orchestrator in front of you and there is none behind you.

Do the work yourself. Do NOT delegate. Do NOT call the Task tool. Do NOT launch subagents. There is no clean context here to protect, and handing the work on would only lose the request.

Read your skill at <alfred>/skills/alfred/SKILL.md and follow it exactly. Read the shared protocols it references. What you were asked to do is run the workflows operation of that skill and report what it prints; the request you were started with is the input to it. Return only what your skill's completion section specifies.

You belong to no workflow. Do not propose a route, do not read a workflow's definition or its rules file, and do not run a phase of one. When the request needs a workflow, finish what this command is for, then name the command that runs the workflow and stop.

## The request

$ARGUMENTS
