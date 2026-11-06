import * as TabsPrimitive from '@radix-ui/react-tabs';
import type { ReactNode } from 'react';

// Tabs wraps Radix Tabs with the segmented HA style.

export const Tabs = TabsPrimitive.Root;

export function TabsList({ children, className = '' }: { children: ReactNode; className?: string }) {
  return (
    <TabsPrimitive.List
      className={`inline-flex items-center gap-0.5 rounded-md border border-edge bg-white/4 p-0.5 ${className}`}
    >
      {children}
    </TabsPrimitive.List>
  );
}

export function TabsTrigger({
  value,
  children,
}: {
  value: string;
  children: ReactNode;
}) {
  return (
    <TabsPrimitive.Trigger
      value={value}
      className="h-8 rounded-md px-4 text-sm font-medium text-ink-muted transition-colors duration-150 ease-standard hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong data-[state=active]:bg-raised data-[state=active]:text-ink"
    >
      {children}
    </TabsPrimitive.Trigger>
  );
}

export function TabsContent({
  value,
  children,
  className = '',
}: {
  value: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <TabsPrimitive.Content value={value} className={`mt-5 outline-none ${className}`}>
      {children}
    </TabsPrimitive.Content>
  );
}
