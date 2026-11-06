import { cva } from 'class-variance-authority'

export const buttonVariants = cva(
  'inline-flex min-h-11 min-w-11 items-center justify-center gap-2 rounded-[2px] border font-mono text-xs font-medium uppercase leading-tight tracking-[0.09em] whitespace-normal text-center no-underline transition-colors duration-150 disabled:pointer-events-none disabled:opacity-45',
  {
    variants: {
      variant: {
        default: 'border-ink bg-ink text-paper hover:border-black hover:bg-black',
        outline:
          'border-ink/30 bg-transparent text-ink hover:border-ink hover:bg-ink/5',
        secondary: 'border-transparent bg-paper text-ink hover:bg-paper',
        ghost: 'border-transparent bg-transparent text-ink hover:bg-ink/5',
        destructive:
          'border-destructive text-destructive hover:bg-destructive hover:text-paper',
        link: 'min-h-11 border-transparent bg-transparent px-0.5 text-ink underline decoration-1 underline-offset-4 hover:decoration-2',
      },
      size: {
        default: 'px-[18px]',
        sm: 'min-h-9 px-3 text-[0.6875rem]',
        lg: 'min-h-12 px-6',
        icon: 'size-11 px-0',
      },
    },
    defaultVariants: { variant: 'default', size: 'default' },
  },
)
