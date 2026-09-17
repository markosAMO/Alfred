/**
 * Alfred skill registry plugin for OpenCode.
 *
 * OpenCode has no session hook in its configuration file; it has plugins, loaded from
 * ~/.config/opencode/plugins/ when a project opens. This one runs `registry.sh sync` for
 * the project, so the registry the orchestrator reads matches the skills on disk. Every
 * rule lives in the script; this file only calls it and forwards its one-line notice to
 * the OpenCode log. Nothing reaches the conversation.
 *
 * Installed by install.sh. Not loaded by Claude Code, which uses a SessionStart hook.
 */
import { existsSync } from "node:fs"
import { homedir } from "node:os"
import { join } from "node:path"

declare const Bun: any

const alfredHome = process.env.ALFRED_HOME ?? join(homedir(), ".config", "alfred")
const script = join(alfredHome, "bin", "registry.sh")

export const AlfredRegistryPlugin = async (input: { directory?: string; worktree?: string }) => {
  const cwd = input.directory ?? input.worktree ?? process.cwd()

  // The script applies the same guards, but skipping here avoids spawning anything for
  // the directories that are not Alfred repositories, which is most of them.
  if (!existsSync(script) || !existsSync(join(cwd, ".alfred"))) return {}

  try {
    const proc = Bun.spawn([script, "sync", "--quiet", "--cwd", cwd], {
      stdout: "pipe",
      stderr: "pipe",
    })
    const [out, err] = await Promise.all([
      new Response(proc.stdout).text(),
      new Response(proc.stderr).text(),
      proc.exited,
    ])
    if (out.trim()) console.error(`[alfred] ${out.trim()}`)
    if (err.trim()) console.error(`[alfred] ${err.trim()}`)
  } catch (error) {
    console.error("[alfred] skill registry sync failed:", error)
  }

  return {}
}
