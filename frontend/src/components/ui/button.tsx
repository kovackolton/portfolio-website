import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";
const buttonVariants = cva("inline-flex items-center justify-center gap-2 rounded-lg text-sm font-medium transition-colors disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-lime-400", { variants: { variant: { default: "bg-lime-300 text-zinc-950 hover:bg-lime-200", outline: "border border-zinc-700 bg-transparent text-zinc-100 hover:bg-zinc-800" }, size: { default: "h-11 px-5", sm: "h-9 px-3" } }, defaultVariants: { variant: "default", size: "default" } });
function Button({ className, variant, size, asChild = false, ...props }: React.ComponentProps<"button"> & VariantProps<typeof buttonVariants> & { asChild?: boolean }) { const Comp = asChild ? Slot : "button"; return <Comp className={cn(buttonVariants({ variant, size, className }))} {...props} />; }
export { Button, buttonVariants };
