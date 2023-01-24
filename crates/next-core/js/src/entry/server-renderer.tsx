// IPC need to be the first import to allow it to catch errors happening during
// the other imports
import startHandler from "@vercel/turbopack-next/internal/page-server-handler";

import App from "@vercel/turbopack-next/pages/_app";
import Document from "@vercel/turbopack-next/pages/_document";

import Component, * as otherExports from ".";
import * as notFoundModule from "@vercel/turbopack-next/pages/404";
import * as errorModule from "next/error";
("TURBOPACK { transition: next-client }");
import chunkGroup from ".";

startHandler({
  isDataReq: false,
  App,
  Document,
  Component,
  otherExports,
  notFoundModule,
  // NOTE(alexkirsz) Can't compare __turbopack_module_id__ of the two modules because the default next/error module is
  // always an external, and as such does not have an id.
  hasCustomNotFound: notFoundModule !== errorModule,
  chunkGroup,
});
