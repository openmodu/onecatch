import * as React from "react";
import { Check } from "lucide-react";
import { Checkbox as CheckboxPrimitive } from "radix-ui";
import { cn } from "@/lib/utils";

function Checkbox({ className, ...props }) {
  return <CheckboxPrimitive.Root data-slot="checkbox" className={cn("inline-flex size-4 shrink-0 items-center justify-center rounded-[4px] border border-input bg-transparent p-0 shadow-none outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/40 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground", className)} {...props}>
    <CheckboxPrimitive.Indicator className="flex items-center justify-center"><Check className="size-3" strokeWidth={2.5} /></CheckboxPrimitive.Indicator>
  </CheckboxPrimitive.Root>;
}

export { Checkbox };
