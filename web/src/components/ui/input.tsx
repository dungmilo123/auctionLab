import * as React from "react"
import { cn } from "cn"
import { fieldClasses } from "./field-styles"

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(fieldClasses, className)}
      {...props}
    />
  )
}

export { Input }
