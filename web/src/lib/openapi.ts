/* eslint-disable */
/* tslint:disable */
// @ts-nocheck
/*
 * ---------------------------------------------------------------
 * ## THIS FILE WAS GENERATED VIA SWAGGER-TYPESCRIPT-API        ##
 * ##                                                           ##
 * ## AUTHOR: acacode                                           ##
 * ## SOURCE: https://github.com/acacode/swagger-typescript-api ##
 * ---------------------------------------------------------------
 */

export interface GithubComBranchdDevBranchdInternalModelsAnonRule {
  column?: string;
  created_at?: string;
  id?: string;
  table?: string;
  template?: string;
}

export interface GithubComBranchdDevBranchdInternalModelsBranch {
  created_at?: string;
  created_by?: GithubComBranchdDevBranchdInternalModelsUser;
  created_by_id?: string;
  id?: string;
  name?: string;
  password?: string;
  port?: number;
  restore?: GithubComBranchdDevBranchdInternalModelsRestore;
  restore_id?: string;
  user?: string;
}

export interface GithubComBranchdDevBranchdInternalModelsRestore {
  branches?: GithubComBranchdDevBranchdInternalModelsBranch[];
  created_at?: string;
  data_ready?: boolean;
  id?: string;
  name?: string;
  port?: number;
  ready_at?: string;
  schema_only?: boolean;
  schema_ready?: boolean;
}

export interface GithubComBranchdDevBranchdInternalModelsUser {
  created_at?: string;
  email?: string;
  id?: string;
  is_admin?: boolean;
  name?: string;
  updated_at?: string;
}

export interface InternalServerBranchListResponse {
  connection_url?: string;
  created_at?: string;
  created_by?: string;
  id?: string;
  name?: string;
  port?: number;
  restore_id?: string;
  restore_name?: string;
}

export interface InternalServerConfigResponse {
  branch_postgresql_conf?: string;
  connection_string?: string;
  created_at?: string;
  database_name?: string;
  domain?: string;
  id?: string;
  last_refreshed_at?: string;
  lets_encrypt_email?: string;
  max_restores?: number;
  next_refresh_at?: string;
  postgres_version?: string;
  refresh_schedule?: string;
  schema_only?: boolean;
}

export interface InternalServerCreateAnonRuleRequest {
  column: string;
  table: string;
  template: string;
}

export interface InternalServerCreateBranchRequest {
  name: string;
}

export interface InternalServerCreateBranchResponse {
  database?: string;
  host?: string;
  id?: string;
  password?: string;
  port?: number;
  user?: string;
}

export interface InternalServerCreateUserRequest {
  email: string;
  is_admin?: boolean;
  name: string;
  password: string;
}

export interface InternalServerCreateUserResponse {
  user?: InternalServerUserDetail;
}

export interface InternalServerDatabaseMetrics {
  connected?: boolean;
  error?: string;
  major_version?: number;
  name?: string;
  size_gb?: number;
  version?: string;
}

export interface InternalServerLatestVersionResponse {
  current_version?: string;
  latest_version?: string;
  update_available?: boolean;
}

export interface InternalServerLoginRequest {
  email: string;
  password: string;
}

export interface InternalServerLoginResponse {
  token?: string;
  user?: InternalServerUserDetail;
}

export interface InternalServerSetupRequest {
  email: string;
  name: string;
  password: string;
}

export interface InternalServerSystemInfoResponse {
  source_database?: InternalServerDatabaseMetrics;
  version?: string;
  vm?: InternalServerVMMetrics;
}

export interface InternalServerUpdateConfigRequest {
  connectionString?: string;
  domain?: string;
  letsEncryptEmail?: string;
  maxRestores?: number;
  postgresVersion?: string;
  refreshSchedule?: string;
  schemaOnly?: boolean;
}

export interface InternalServerUserDetail {
  created_at?: string;
  email?: string;
  id?: string;
  is_admin?: boolean;
  name?: string;
}

export interface InternalServerVMMetrics {
  cpu_count?: number;
  disk_available_gb?: number;
  disk_total_gb?: number;
  disk_used_gb?: number;
  disk_used_percent?: number;
  memory_free_gb?: number;
  memory_total_gb?: number;
  memory_used_gb?: number;
}

export type QueryParamsType = Record<string | number, any>;
export type ResponseFormat = keyof Omit<Body, "body" | "bodyUsed">;

export interface FullRequestParams extends Omit<RequestInit, "body"> {
  secure?: boolean;
  path: string;
  type?: ContentType;
  query?: QueryParamsType;
  format?: ResponseFormat;
  body?: unknown;
  baseUrl?: string;
  cancelToken?: CancelToken;
}

export type RequestParams = Omit<
  FullRequestParams,
  "body" | "method" | "query" | "path"
>;

export interface ApiConfig<SecurityDataType = unknown> {
  baseUrl?: string;
  baseApiParams?: Omit<RequestParams, "baseUrl" | "cancelToken" | "signal">;
  securityWorker?: (
    securityData: SecurityDataType | null,
  ) => Promise<RequestParams | void> | RequestParams | void;
  customFetch?: typeof fetch;
}

export interface HttpResponse<D extends unknown, E extends unknown = unknown>
  extends Response {
  data: D;
  error: E;
}

type CancelToken = Symbol | string | number;

export enum ContentType {
  Json = "application/json",
  JsonApi = "application/vnd.api+json",
  FormData = "multipart/form-data",
  UrlEncoded = "application/x-www-form-urlencoded",
  Text = "text/plain",
}

export class HttpClient<SecurityDataType = unknown> {
  public baseUrl: string = "//localhost:8080";
  private securityData: SecurityDataType | null = null;
  private securityWorker?: ApiConfig<SecurityDataType>["securityWorker"];
  private abortControllers = new Map<CancelToken, AbortController>();
  private customFetch = (...fetchParams: Parameters<typeof fetch>) =>
    fetch(...fetchParams);

  private baseApiParams: RequestParams = {
    credentials: "same-origin",
    headers: {},
    redirect: "follow",
    referrerPolicy: "no-referrer",
  };

  constructor(apiConfig: ApiConfig<SecurityDataType> = {}) {
    Object.assign(this, apiConfig);
  }

  public setSecurityData = (data: SecurityDataType | null) => {
    this.securityData = data;
  };

  protected encodeQueryParam(key: string, value: any) {
    const encodedKey = encodeURIComponent(key);
    return `${encodedKey}=${encodeURIComponent(typeof value === "number" ? value : `${value}`)}`;
  }

  protected addQueryParam(query: QueryParamsType, key: string) {
    return this.encodeQueryParam(key, query[key]);
  }

  protected addArrayQueryParam(query: QueryParamsType, key: string) {
    const value = query[key];
    return value.map((v: any) => this.encodeQueryParam(key, v)).join("&");
  }

  protected toQueryString(rawQuery?: QueryParamsType): string {
    const query = rawQuery || {};
    const keys = Object.keys(query).filter(
      (key) => "undefined" !== typeof query[key],
    );
    return keys
      .map((key) =>
        Array.isArray(query[key])
          ? this.addArrayQueryParam(query, key)
          : this.addQueryParam(query, key),
      )
      .join("&");
  }

  protected addQueryParams(rawQuery?: QueryParamsType): string {
    const queryString = this.toQueryString(rawQuery);
    return queryString ? `?${queryString}` : "";
  }

  private contentFormatters: Record<ContentType, (input: any) => any> = {
    [ContentType.Json]: (input: any) =>
      input !== null && (typeof input === "object" || typeof input === "string")
        ? JSON.stringify(input)
        : input,
    [ContentType.JsonApi]: (input: any) =>
      input !== null && (typeof input === "object" || typeof input === "string")
        ? JSON.stringify(input)
        : input,
    [ContentType.Text]: (input: any) =>
      input !== null && typeof input !== "string"
        ? JSON.stringify(input)
        : input,
    [ContentType.FormData]: (input: any) => {
      if (input instanceof FormData) {
        return input;
      }

      return Object.keys(input || {}).reduce((formData, key) => {
        const property = input[key];
        formData.append(
          key,
          property instanceof Blob
            ? property
            : typeof property === "object" && property !== null
              ? JSON.stringify(property)
              : `${property}`,
        );
        return formData;
      }, new FormData());
    },
    [ContentType.UrlEncoded]: (input: any) => this.toQueryString(input),
  };

  protected mergeRequestParams(
    params1: RequestParams,
    params2?: RequestParams,
  ): RequestParams {
    return {
      ...this.baseApiParams,
      ...params1,
      ...(params2 || {}),
      headers: {
        ...(this.baseApiParams.headers || {}),
        ...(params1.headers || {}),
        ...((params2 && params2.headers) || {}),
      },
    };
  }

  protected createAbortSignal = (
    cancelToken: CancelToken,
  ): AbortSignal | undefined => {
    if (this.abortControllers.has(cancelToken)) {
      const abortController = this.abortControllers.get(cancelToken);
      if (abortController) {
        return abortController.signal;
      }
      return void 0;
    }

    const abortController = new AbortController();
    this.abortControllers.set(cancelToken, abortController);
    return abortController.signal;
  };

  public abortRequest = (cancelToken: CancelToken) => {
    const abortController = this.abortControllers.get(cancelToken);

    if (abortController) {
      abortController.abort();
      this.abortControllers.delete(cancelToken);
    }
  };

  public request = async <T = any, E = any>({
    body,
    secure,
    path,
    type,
    query,
    format,
    baseUrl,
    cancelToken,
    ...params
  }: FullRequestParams): Promise<HttpResponse<T, E>> => {
    const secureParams =
      ((typeof secure === "boolean" ? secure : this.baseApiParams.secure) &&
        this.securityWorker &&
        (await this.securityWorker(this.securityData))) ||
      {};
    const requestParams = this.mergeRequestParams(params, secureParams);
    const queryString = query && this.toQueryString(query);
    const payloadFormatter = this.contentFormatters[type || ContentType.Json];
    const responseFormat = format || requestParams.format;

    return this.customFetch(
      `${baseUrl || this.baseUrl || ""}${path}${queryString ? `?${queryString}` : ""}`,
      {
        ...requestParams,
        headers: {
          ...(requestParams.headers || {}),
          ...(type && type !== ContentType.FormData
            ? { "Content-Type": type }
            : {}),
        },
        signal:
          (cancelToken
            ? this.createAbortSignal(cancelToken)
            : requestParams.signal) || null,
        body:
          typeof body === "undefined" || body === null
            ? null
            : payloadFormatter(body),
      },
    ).then(async (response) => {
      const r = response as HttpResponse<T, E>;
      r.data = null as unknown as T;
      r.error = null as unknown as E;

      const responseToParse = responseFormat ? response.clone() : response;
      const data = !responseFormat
        ? r
        : await responseToParse[responseFormat]()
            .then((data) => {
              if (r.ok) {
                r.data = data;
              } else {
                r.error = data;
              }
              return r;
            })
            .catch((e) => {
              r.error = e;
              return r;
            });

      if (cancelToken) {
        this.abortControllers.delete(cancelToken);
      }

      if (!response.ok) throw data;
      return data;
    });
  };
}

export class Api<
  SecurityDataType extends unknown,
> extends HttpClient<SecurityDataType> {
  api = {
    anonRulesList: (params: RequestParams = {}) =>
      this.request<GithubComBranchdDevBranchdInternalModelsAnonRule[], any>({
        path: `/api/anon-rules`,
        method: "GET",
        ...params,
      }),

    anonRulesCreate: (
      request: InternalServerCreateAnonRuleRequest,
      params: RequestParams = {},
    ) =>
      this.request<GithubComBranchdDevBranchdInternalModelsAnonRule, any>({
        path: `/api/anon-rules`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        ...params,
      }),

    anonRulesDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/anon-rules/${id}`,
        method: "DELETE",
        ...params,
      }),

    authLoginCreate: (
      request: InternalServerLoginRequest,
      params: RequestParams = {},
    ) =>
      this.request<InternalServerLoginResponse, Record<string, any>>({
        path: `/api/auth/login`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    authMeList: (params: RequestParams = {}) =>
      this.request<InternalServerUserDetail, Record<string, any>>({
        path: `/api/auth/me`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    branchesList: (params: RequestParams = {}) =>
      this.request<InternalServerBranchListResponse[], any>({
        path: `/api/branches`,
        method: "GET",
        ...params,
      }),

    branchesCreate: (
      body: InternalServerCreateBranchRequest,
      params: RequestParams = {},
    ) =>
      this.request<InternalServerCreateBranchResponse, any>({
        path: `/api/branches`,
        method: "POST",
        body: body,
        type: ContentType.Json,
        ...params,
      }),

    branchesIdDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, any>({
        path: `/api/branches/${id}`,
        method: "DELETE",
        ...params,
      }),

    configList: (params: RequestParams = {}) =>
      this.request<InternalServerConfigResponse, Record<string, any>>({
        path: `/api/config`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    configPartialUpdate: (
      request: InternalServerUpdateConfigRequest,
      params: RequestParams = {},
    ) =>
      this.request<InternalServerConfigResponse, Record<string, any>>({
        path: `/api/config`,
        method: "PATCH",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    restoresList: (params: RequestParams = {}) =>
      this.request<
        GithubComBranchdDevBranchdInternalModelsRestore[],
        Record<string, any>
      >({
        path: `/api/restores`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    restoresTriggerRestoreCreate: (params: RequestParams = {}) =>
      this.request<Record<string, any>, Record<string, any>>({
        path: `/api/restores/trigger-restore`,
        method: "POST",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    restoresDetail: (id: string, params: RequestParams = {}) =>
      this.request<
        GithubComBranchdDevBranchdInternalModelsRestore,
        Record<string, any>
      >({
        path: `/api/restores/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    restoresDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, Record<string, any>>({
        path: `/api/restores/${id}`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    setupCreate: (
      request: InternalServerSetupRequest,
      params: RequestParams = {},
    ) =>
      this.request<InternalServerLoginResponse, Record<string, any>>({
        path: `/api/setup`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    systemInfoList: (params: RequestParams = {}) =>
      this.request<InternalServerSystemInfoResponse, Record<string, any>>({
        path: `/api/system/info`,
        method: "GET",
        format: "json",
        ...params,
      }),

    systemLatestVersionList: (params: RequestParams = {}) =>
      this.request<InternalServerLatestVersionResponse, Record<string, any>>({
        path: `/api/system/latest-version`,
        method: "GET",
        format: "json",
        ...params,
      }),

    systemUpdateCreate: (params: RequestParams = {}) =>
      this.request<Record<string, any>, Record<string, any>>({
        path: `/api/system/update`,
        method: "POST",
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    usersList: (params: RequestParams = {}) =>
      this.request<InternalServerUserDetail[], Record<string, any>>({
        path: `/api/users`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    usersCreate: (
      request: InternalServerCreateUserRequest,
      params: RequestParams = {},
    ) =>
      this.request<InternalServerCreateUserResponse, Record<string, any>>({
        path: `/api/users`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    usersDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, Record<string, any>>({
        path: `/api/users/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),
  };
  health = {
    healthList: (params: RequestParams = {}) =>
      this.request<Record<string, any>, any>({
        path: `/health`,
        method: "GET",
        ...params,
      }),
  };
}
