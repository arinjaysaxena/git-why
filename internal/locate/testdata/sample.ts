import { x } from "./x";

/** A greeter. */
export class Greeter<T> extends Base {
  private name = "}{ not a brace";

  constructor(private readonly greeting: string) {
    super();
  }

  @memo()
  async greet(who: string): Promise<string> {
    if (who) {
      return `${this.greeting}, ${who}`; // }
    }
    return "";
  }

  static create(): Greeter<string> { return new Greeter("hi"); }

  handler = (e: Event) => {
    console.log(e);
  };
}

export interface Options {
  retries: number;
  onDone(code: number): void;
}

export type Id = string | number;

export function parse(input: string): number;
export function parse(input: number): number;
export function parse(input: string | number): number {
  return Number(input);
}

export const double = (n: number) => n * 2;

export const triple = async (n: number) => {
  return n * 3;
};

const add = (a: number,
             b: number) =>
  a + b;

function* gen() {
  yield 1;
}
