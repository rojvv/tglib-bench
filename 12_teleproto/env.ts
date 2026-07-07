import { cleanEnv, num, str, url } from "envalid";

export default cleanEnv(process.env, {
  API_ID: num(),
  API_HASH: str(),
  AUTH_STRING: str(),
  MESSAGE_LINK: url(),
  CHAT_ID: num(),
});
