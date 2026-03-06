import { describe, it, expect } from "vitest";
import { queryKeys } from "../query-keys";

describe("queryKeys.threads", () => {
  it("all has expected shape", () => {
    expect(queryKeys.threads.all).toEqual(["threads"]);
  });

  it("lists extends all", () => {
    const lists = queryKeys.threads.lists();
    expect(lists[0]).toBe("threads");
    expect(lists[1]).toBe("list");
    expect(lists.length).toBe(2);
  });

  it("list includes domain, label, and page", () => {
    const key = queryKeys.threads.list("d1", "inbox", 1);
    expect(key).toEqual(["threads", "list", "d1", "inbox", 1]);
  });

  it("details extends all", () => {
    const details = queryKeys.threads.details();
    expect(details[0]).toBe("threads");
    expect(details[1]).toBe("detail");
  });

  it("detail includes threadId", () => {
    const key = queryKeys.threads.detail("t123");
    expect(key).toEqual(["threads", "detail", "t123"]);
  });
});

describe("queryKeys.search", () => {
  it("all has expected shape", () => {
    expect(queryKeys.search.all).toEqual(["search"]);
  });

  it("results includes domain and query", () => {
    const key = queryKeys.search.results("d1", "hello");
    expect(key).toEqual(["search", "d1", "hello"]);
  });
});

describe("queryKeys.drafts", () => {
  it("all has expected shape", () => {
    expect(queryKeys.drafts.all).toEqual(["drafts"]);
  });

  it("list includes domainId", () => {
    const key = queryKeys.drafts.list("d1");
    expect(key).toEqual(["drafts", "d1"]);
  });
});

describe("queryKeys.domains", () => {
  it("all has expected shape", () => {
    expect(queryKeys.domains.all).toEqual(["domains"]);
  });

  it("list extends all", () => {
    const key = queryKeys.domains.list();
    expect(key).toEqual(["domains", "list"]);
  });

  it("unreadCounts extends all", () => {
    const key = queryKeys.domains.unreadCounts();
    expect(key).toEqual(["domains", "unreadCounts"]);
  });
});

describe("queryKeys uniqueness", () => {
  it("different thread list params produce different keys", () => {
    const k1 = queryKeys.threads.list("d1", "inbox", 1);
    const k2 = queryKeys.threads.list("d1", "inbox", 2);
    const k3 = queryKeys.threads.list("d2", "inbox", 1);
    expect(k1).not.toEqual(k2);
    expect(k1).not.toEqual(k3);
  });

  it("thread detail keys differ by threadId", () => {
    const k1 = queryKeys.threads.detail("t1");
    const k2 = queryKeys.threads.detail("t2");
    expect(k1).not.toEqual(k2);
  });

  it("search results keys differ by query", () => {
    const k1 = queryKeys.search.results("d1", "hello");
    const k2 = queryKeys.search.results("d1", "world");
    expect(k1).not.toEqual(k2);
  });
});
