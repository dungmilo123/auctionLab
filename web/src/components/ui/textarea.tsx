import * as React from "react"
import { cn } from "cn"
import { fieldClasses } from "./field-styles"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(fieldClasses, "min-h-24 resize-y py-2.5", className)}
      {...props}
    />
  )
}

export { Textarea }
