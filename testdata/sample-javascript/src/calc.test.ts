import { describe, expect, it } from "vitest";
import { add, subtract } from "./calc";

describe("calculator", () => {
  it("adds values", () => {
    expect(add(2, 3)).toBe(5);
  });

  it("subtracts values", () => {
    expect(subtract(5, 3)).toBe(2);
  });
});
