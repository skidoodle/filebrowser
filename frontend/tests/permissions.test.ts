import { describe, expect, test } from "bun:test";
import { canWriteIn, canWritePath } from "../src/lib/permissions";
import { setEditToken, getEditToken, hasEditToken, clearEditToken } from "../src/lib/tokens";
import type { Me } from "../src/api/auth";

describe("permissions", () => {
  test("canWriteIn under public policy", () => {
    const anonPublic: Me = { admin: false, insecure: false, initialized: true, access_policy: "public" };
    expect(canWriteIn(anonPublic, "any/folder")).toBe(true);
    expect(canWriteIn(anonPublic, ".")).toBe(true);

    const anonReadonly: Me = { admin: false, insecure: false, initialized: true, access_policy: "readonly" };
    expect(canWriteIn(anonReadonly, "any/folder")).toBe(false);

    const anonPrivate: Me = { admin: false, insecure: false, initialized: true, access_policy: "private" };
    expect(canWriteIn(anonPrivate, "any/folder")).toBe(false);

    const user: Me = { admin: false, insecure: false, initialized: true, username: "alice", scope: "alice" };
    expect(canWriteIn(user, "alice/sub")).toBe(true);
    expect(canWriteIn(user, "other")).toBe(false);
  });

  test("canWritePath denies anonymous", () => {
    const anonPublic: Me = { admin: false, insecure: false, initialized: true, access_policy: "public" };
    expect(canWritePath(anonPublic, "any/file.txt")).toBe(false);
  });
});

describe("edit tokens", () => {
  test("stores, retrieves, and clears tokens", () => {
    setEditToken("notes.txt", "tok123");
    expect(hasEditToken("notes.txt")).toBe(true);
    expect(getEditToken("notes.txt")).toBe("tok123");
    expect(hasEditToken("other.txt")).toBe(false);
    clearEditToken("notes.txt");
    expect(hasEditToken("notes.txt")).toBe(false);
  });
});
