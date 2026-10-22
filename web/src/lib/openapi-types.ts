export type ApiField = {
  name: string;
  type: string;
  required: boolean;
  description: string;
  constraints: string[];
};

export type ApiParam = {
  name: string;
  in: string;
  required: boolean;
  type: string;
  description: string;
  constraints: string[];
};

export type ApiResponse = {
  code: string;
  description: string;
  schema: string;
};

export type ApiEndpoint = {
  id: string;
  method: string;
  path: string;
  summary: string;
  params: ApiParam[];
  body: { schema: string; fields: ApiField[] } | null;
  responses: ApiResponse[];
};

export type ApiSchema = {
  name: string;
  fields: ApiField[];
};

export type ApiReference = {
  title: string;
  version: string;
  description: string;
  auth: string;
  endpoints: ApiEndpoint[];
  schemas: ApiSchema[];
  errorCodes: string[];
  eventTypes: string[];
  permissions: string[];
};
