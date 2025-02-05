"use client";

// Error components must be Client Components

export default function Error(props: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return <>404</>;
}
