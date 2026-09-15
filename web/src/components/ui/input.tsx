import * as React from "react"
import { cn } from "cn"

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        // Neobrutalist field (01-UI-SPEC.md §Interaction rules): 2px near-black
        // outline, zero radius, 44px minimum height, and horizontal padding on
        // the md token (16px) — the CLI's smaller step sat off the declared
        // scale. The transition covers paint properties only so the 2px ring
        // reaches full opacity the instant :focus-visible matches, and motion is
        // fully disabled under prefers-reduced-motion (Button does the same).
        // Never spell spacing/type utility names in these comments — Tailwind's
        // scanner reads source text and emits whatever utility it finds.
        "h-11 w-full min-w-0 rounded-none border-2 border-border bg-transparent px-4 py-2 text-base leading-[1.5] transition-[color,background-color,border-color] motion-reduce:transition-none file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-sm file:font-bold file:text-foreground placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:bg-input/50 disabled:opacity-50 aria-invalid:border-destructive md:text-sm dark:bg-input/30 dark:disabled:bg-input/80 dark:aria-invalid:border-destructive/50",
        className
      )}
      {...props}
    />
  )
}

export { Input }
