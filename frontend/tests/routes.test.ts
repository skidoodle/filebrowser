import { describe, expect, test } from "bun:test";
import { addBasePath, removeBasePath } from "../src/lib/base";
import { isReservedPath, parseRoute, routePath, type AppRoute } from "../src/lib/routes";

describe("isReservedPath", () => {
  test("claims route-named roots only", () => {
    for (const root of ["view", "new", "settings", "login", "setup"]) {
      expect(isReservedPath(root)).toBe(true);
      expect(isReservedPath(`${root}/notes.txt`)).toBe(true);
    }
    expect(isReservedPath("docs/view")).toBe(false);
    expect(isReservedPath("views")).toBe(false);
    expect(isReservedPath(".")).toBe(false);
  });
});

describe("base paths", () => {
  test("adds and removes an application mount path", () => {
    expect(addBasePath("/share", "/")).toBe("/share/");
    expect(addBasePath("/share/", "/view/docs/readme.txt")).toBe("/share/view/docs/readme.txt");
    expect(removeBasePath("/share", "/share/settings/users")).toBe("/settings/users");
    expect(removeBasePath("/share", "/settings/users")).toBeNull();
  });
});

describe("routePath", () => {
  test("creates readable, segment-encoded paths", () => {
    expect(routePath({ page: "files", dir: "." })).toBe("/");
    expect(routePath({ page: "files", dir: "sample image/100% #1" })).toBe(
      "/sample%20image/100%25%20%231",
    );
    expect(routePath({ page: "viewer", path: "sample image/sample.webp", dir: "sample image" })).toBe(
      "/view/sample%20image/sample.webp",
    );
    expect(routePath({ page: "settings", tab: "users", dir: "sample image" })).toBe("/settings/users");
    expect(routePath({ page: "create", kind: "dir", dir: "sample image" })).toBe(
      "/new/folder/sample%20image",
    );
  });

  test("round-trips every canonical route", () => {
    const routes: AppRoute[] = [
      { page: "files", dir: "." },
      { page: "files", dir: "docs/C++ notes" },
      { page: "viewer", path: "docs/guide.pdf", dir: "inbox" },
      { page: "create", kind: "file", dir: "docs" },
      { page: "settings", tab: "profile", dir: "docs" },
      { page: "settings", tab: "policy", dir: "docs" },
      { page: "auth", mode: "login", dir: "docs" },
    ];

    for (const route of routes) {
      expect(parseRoute(routePath(route), route.dir)).toEqual(route);
    }
  });
});

describe("parseRoute", () => {
  test("treats unreserved paths as directory routes", () => {
    expect(parseRoute("/sample%20image")).toEqual({ page: "files", dir: "sample image" });
    expect(parseRoute("/docs/sub")).toEqual({ page: "files", dir: "docs/sub" });
  });

  test("derives a viewer background from direct deep links", () => {
    expect(parseRoute("/view/sample%20image/sample.webp")).toEqual({
      page: "viewer",
      path: "sample image/sample.webp",
      dir: "sample image",
    });
  });

  test("retains the current directory when history provides one", () => {
    expect(parseRoute("/settings/users", "sample image")).toEqual({
      page: "settings",
      tab: "users",
      dir: "sample image",
    });
    expect(parseRoute("/view/archive/photo.jpg", "recent")).toEqual({
      page: "viewer",
      path: "archive/photo.jpg",
      dir: "recent",
    });
  });

  test("falls back safely for malformed routes", () => {
    expect(parseRoute("/view/%E0%A4%A")).toEqual({ page: "files", dir: "." });
    expect(parseRoute("/%E0%A4%A")).toEqual({ page: "files", dir: "." });
  });
});
