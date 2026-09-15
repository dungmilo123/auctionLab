import * as React from "react"
import { cn } from "cn"
import { Label as LabelPrimitive } from "radix-ui"

function Label({
  className,
  ...props
}: React.ComponentProps<typeof LabelPrimitive.Root>) {
  return (
    <LabelPrimitive.Root
      data-slot="label"
      className={cn(
        // 01-UI-SPEC.md §Typography: label is 14px at weight 700 — the token
        // scale has exactly two weights (400/700), so the CLI's medium step
        // goes. Never write that utility name here: Tailwind's scanner reads
        // source comments, so the literal string emits a weight-500 rule into
        // every production build.
        "flex items-center gap-2 text-sm leading-[1.5] font-bold select-none group-data-[disabled=true]:pointer-events-none group-data-[disabled=true]:opacity-50 peer-disabled:cursor-not-allowed peer-disabled:opacity-50",
        className
      )}
      {...props}
    />
  )
}

export { Label }
