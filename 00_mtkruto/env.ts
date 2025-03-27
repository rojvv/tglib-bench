import { cleanEnv, num, str, url } from "envalid";

export default cleanEnv(Deno.env.toObject(), {
  AUTH_STRING: str(),
  MESSAGE_LINK: url(),
  CHAT_ID: num(),
});
