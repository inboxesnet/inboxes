import "@testing-library/jest-dom/vitest";

// Node 22+ ships an experimental global localStorage whose methods throw
// unless Node is started with --localstorage-file. It shadows jsdom's
// implementation, so give tests a real in-memory Storage instead.
class MemoryStorage implements Storage {
  private store = new Map<string, string>();
  get length() {
    return this.store.size;
  }
  clear() {
    this.store.clear();
  }
  getItem(key: string) {
    return this.store.has(key) ? this.store.get(key)! : null;
  }
  key(index: number) {
    return Array.from(this.store.keys())[index] ?? null;
  }
  removeItem(key: string) {
    this.store.delete(key);
  }
  setItem(key: string, value: string) {
    this.store.set(key, String(value));
  }
}

const needsPolyfill = (() => {
  try {
    globalThis.localStorage.setItem("__probe__", "1");
    globalThis.localStorage.removeItem("__probe__");
    return typeof globalThis.localStorage.clear !== "function";
  } catch {
    return true;
  }
})();

if (needsPolyfill) {
  Object.defineProperty(globalThis, "localStorage", {
    value: new MemoryStorage(),
    configurable: true,
    writable: true,
  });
  Object.defineProperty(globalThis, "sessionStorage", {
    value: new MemoryStorage(),
    configurable: true,
    writable: true,
  });
}
