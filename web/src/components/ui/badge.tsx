import * as React from "react"
import { cn } from "cn"
import { cva, type VariantProps } from "class-variance-authority"

const badgeVariants = cva(
  "inline-flex items-center gap-1.5 font-mono text-[0.625rem] font-medium tracking-[0.12em] uppercase text-ink",
  {
    variants: {
      tone: {
        neutral: "",
        live: "",
        closing: "",
        upcoming: "",
        ended: "text-muted-foreground",
        sold: "text-muted-foreground",
      },
    },
    defaultVariants: { tone: "neutral" },
  },
)

const dotVariants = cva("size-[7px] shrink-0 rounded-full", {
  variants: {
    tone: {
      neutral: "bg-ink",
      live: "bg-[#2C6A50]",
      closing: "bg-[#8A6A12]",
      upcoming: "bg-[#6A5A8C]",
      ended: "bg-[#9C9C96]",
      sold: "bg-[#9C9C96]",
    },
  },
  defaultVariants: { tone: "neutral" },
})

function Badge({
  className,
  tone,
  dot = true,
  children,
  ...props
}: React.ComponentProps<"span"> &
  VariantProps<typeof badgeVariants> & { dot?: boolean }) {
  return (
    <span
      data-slot="badge"
      className={cn(badgeVariants({ tone }), className)}
      {...props}
    >
      {dot && (
        <span aria-hidden="true" className={cn(dotVariants({ tone }))} />
      )}
      {children}
    </span>
  )
}

export { Badge }
