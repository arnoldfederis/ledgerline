#!/usr/bin/env bun
import { Glob } from "bun";
import { readFileSync, writeFileSync } from "node:fs";
import { argv, exit } from "node:process";
import { format } from "sql-formatter";

// Formats every .sql file in the repo, the way oxfmt formats the TypeScript.
//
//   bun tools/scripts/format-sql.mjs          rewrite files in place
//   bun tools/scripts/format-sql.mjs --check  exit non-zero if anything differs
//
// paramTypes.named keeps sqlc's @param placeholders intact; without it the
// formatter inserts a space and sqlc stops recognising them.
const OPTIONS = {
  language: "postgresql",
  tabWidth: 2,
  useTabs: false,
  keywordCase: "upper",
  expressionWidth: 80,
  linesBetweenQueries: 1,
  paramTypes: { named: ["@"] },
};

const IGNORED = ["node_modules", ".devbox", ".direnv", ".turbo", "dist", "build"];

// The formatter treats sqlc.arg/sqlc.narg as ordinary function calls and puts a
// space before the paren. sqlc copes, but it reads badly.
function tidySqlcCalls(sql) {
  return sql.replaceAll(/\bsqlc\.(n?arg) \(/g, "sqlc.$1(");
}

const check = argv.includes("--check");
const glob = new Glob("**/*.sql");

let changed = 0;
let scanned = 0;

for await (const file of glob.scan({ cwd: ".", onlyFiles: true })) {
  if (IGNORED.some((dir) => file.split("/").includes(dir))) continue;

  scanned += 1;
  const original = readFileSync(file, "utf8");
  const formatted = `${tidySqlcCalls(format(original, OPTIONS)).trimEnd()}\n`;

  if (original === formatted) continue;

  changed += 1;
  if (check) {
    console.error(`needs formatting: ${file}`);
  } else {
    writeFileSync(file, formatted);
    console.log(`formatted: ${file}`);
  }
}

if (check && changed > 0) {
  console.error(`\n${changed} of ${scanned} SQL files need formatting. Run: bun run format:sql`);
  exit(1);
}

console.log(check ? `${scanned} SQL files formatted correctly` : `${scanned} SQL files checked`);
