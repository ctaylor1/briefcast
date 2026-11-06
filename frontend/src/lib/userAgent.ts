export const USER_AGENT_ENV = "BRIEFCAST_USER_AGENT";

/** Explains where the outbound User-Agent shown in Settings comes from. */
export function userAgentSourceLabel(source: string | undefined): string {
  switch (source) {
    case "env":
      return `Set by ${USER_AGENT_ENV} in .env.`;
    case "legacy_setting":
      return `Saved by the legacy settings page. Set ${USER_AGENT_ENV} in .env to replace it.`;
    default:
      return `Briefcast default. Set ${USER_AGENT_ENV} in .env and restart to change it.`;
  }
}
