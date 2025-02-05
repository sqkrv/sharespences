import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

export function middleware(request: NextRequest) {}

// Применяем middleware только к корневому маршруту "/"
export const config = {
  matcher: "/",
};
