import { cva } from "class-variance-authority"

// Variant recipe for the Button primitive. It lives in its own module because
// oxlint's react(only-export-components) warning (Fast Refresh) fires when a
// component module also exports a constant — the project's own
// `allowConstantExport: true` does not silence it. Import it from here, never
// re-export it from button.tsx.
//
// Neobrutalist affordance (01-UI-SPEC.md §Interaction rules): 2px near-black
// outline, zero radius, hard offset shadow, 44px minimum hit target, active
// press translating (2px, 2px) with the shadow collapsing, disabled at 50%
// opacity with no shadow, and all motion disabled under prefers-reduced-motion.
// The CLI's soft/rounded defaults must not ship as-is (§Component Inventory).
export const buttonVariants = cva(
  "group/button inline-flex min-h-11 min-w-11 shrink-0 items-center justify-center rounded-none border-2 border-border bg-clip-padding text-sm font-bold whitespace-nowrap shadow-(--shadow-hard) transition-[transform,box-shadow,background-color] duration-[80ms] select-none motion-reduce:transition-none active:not-aria-[haspopup]:translate-x-[2px] active:not-aria-[haspopup]:translate-y-[2px] active:not-aria-[haspopup]:shadow-none disabled:cursor-not-allowed disabled:opacity-50 disabled:shadow-none aria-invalid:border-destructive dark:aria-invalid:border-destructive/50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-accent",
        outline:
          "border-border bg-background hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground dark:border-input dark:bg-input/30 dark:hover:bg-input/50",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-accent aria-expanded:bg-secondary aria-expanded:text-secondary-foreground",
        ghost:
          "hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground dark:hover:bg-muted/50",
        destructive:
          "bg-destructive/10 text-destructive hover:bg-destructive/20 dark:bg-destructive/20 dark:hover:bg-destructive/30",
        link: "text-primary underline-offset-4 hover:underline",
      },
      // Only the default size ships (01-UI-SPEC.md §Typography/§Spacing): the
      // CLI's smaller steps put a 12px and a 12.8px font size into every
      // production build, and neither is one of the four declared type roles.
      // Never spell those utility names out in prose either — Tailwind's
      // scanner reads source comments and would emit the rules anyway. The
      // variant key stays so Button's `size` prop keeps its shape.
      size: {
        default:
          "h-11 gap-2 px-4 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)
