import { fetchBaseQuery } from "@reduxjs/toolkit/query/react";

const baseQuery = fetchBaseQuery({
  baseUrl: `${process.env.NEXT_PUBLIC_SERVER_URL}`,
  prepareHeaders: (headers) => {
    return headers;
  },
});

export default baseQuery;
