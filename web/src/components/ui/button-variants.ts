import { cva } from 'class-variance-authority'

export const buttonVariants = cva('site-button', {
  variants: {
    variant: {
      default: 'site-button-primary',
      outline: 'site-button-outline',
      secondary: 'site-button-secondary',
      ghost: 'site-button-ghost',
      destructive: 'site-button-destructive',
      link: 'site-button-link',
    },
    size: { default: '' },
  },
  defaultVariants: { variant: 'default', size: 'default' },
})
