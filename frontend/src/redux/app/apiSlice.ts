import { createApi } from "@reduxjs/toolkit/query/react";
import baseQuery from "../fetchBaseQuery";

export const apiSlice = createApi({
  reducerPath: "api",
  baseQuery: baseQuery,
  endpoints: (builder) => ({}),
});

export const {} = apiSlice;
