import * as React from "react"
import { cn } from "cn"
import { fieldClasses } from "./field-styles"

function Select({ className, children, ...props }: React.ComponentProps<"select">) {
  return (
    <select
      data-slot="select"
      className={cn(fieldClasses, "appearance-none pr-9", className)}
      {...props}
    >
      {children}
    </select>
  )
}

export { Select }
