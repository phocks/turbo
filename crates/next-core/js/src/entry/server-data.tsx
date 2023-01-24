// IPC need to be the first import to allow it to catch errors happening during
// the other imports
import startHandler from "@vercel/turbopack-next/internal/page-server-handler";

// eslint-disable-next-line
import Document from "next/document";
import App from "next/app";
import * as otherExports from ".";
import * as notFoundModule from "@vercel/turbopack-next/pages/404";
import * as errorModule from "next/error";

startHandler({
  isDataReq: true,
  App,
  Document,
  Component: () => {},
  otherExports,
  notFoundModule,
  // NOTE(alexkirsz) Can't compare __turbopack_module_id__ of the two modules because the default next/error module is
  // always an external, and as such does not have an id.
  hasCustomNotFound: notFoundModule !== errorModule,
});
