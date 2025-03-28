#include <td/telegram/Client.h>
#include <td/telegram/td_api.h>
#include <td/telegram/td_api.hpp>

#include <map>
#include <ctime>
#include <cstring>
#include <fstream>
#include <iostream>
#include <functional>

namespace td_api = td::td_api;

class Client {
 public:
  Client() {
    td::ClientManager::execute(td_api::make_object<td_api::setLogVerbosityLevel>(1));
    client_manager_ = std::make_unique<td::ClientManager>();
    client_id_ = client_manager_->create_client_id();
    send_query(td_api::make_object<td_api::getOption>("version"), nullptr);
  }

  void loop() {
    while (true) {
      if (must_quit_) {
        break;
      } else if (!is_authorized_) {
        on_response(client_manager_->receive(10));
      } else if (!fetched_message_) {
        fetch_message();
      } else if (file_id_ && !download_started_) {
        start_download();
      } else if (file_ != nullptr && !upload_started_) {
        start_upload();
      } else if (upload_finished_) {
        write_results();
        break;
      } else {
        on_response(client_manager_->receive(1));
      }
    }
  }

 private:
  std::unique_ptr<td::ClientManager> client_manager_;
  td::ClientManager::ClientId client_id_;

  std::time_t timestamps_[4];
  bool must_quit_{false};
  bool is_authorized_{false};
  bool upload_finished_{false};
  std::uint16_t current_query_id_;
  std::int32_t file_id_{0};
  td::td_api::int53 message_id_ = 0;
  const char *file_name_ = "file";
  td_api::object_ptr<td_api::file> file_;

  std::map<td::ClientManager::RequestId, std::function<void(td_api::object_ptr<td_api::Object>)>> handlers_;

  void send_query(td_api::object_ptr<td_api::Function> f,
                  std::function<void(td_api::object_ptr<td_api::Object>)> handler) {
    auto query_id = ++current_query_id_;
    if (handler) {
      handlers_.emplace(query_id, std::move(handler));
    }
    client_manager_->send(client_id_, query_id, std::move(f));
  }

  void query_must_succeed(td_api::object_ptr<td_api::Function> f) {
    send_query(std::move(f), [this](auto object) {
      if (object->get_id() == td_api::error::ID) {
        auto error = td::move_tl_object_as<td_api::error>(object);
        std::cout << "Error: " << to_string(error) << std::flush;
        must_quit_ = true;
      }
    });
  }

  bool assert_id(int32_t id, td_api::Object *object) {
    if (object->get_id() != id) {
      std::cout << "Got invalid response: " << to_string(*object);
      must_quit_ = true;
      return false;
    } else {
      return true;
    }
  }

  void on_response(td::ClientManager::Response response) {
    if (!response.object) {
      return;
    }
    if (response.request_id == 0) {
      return on_update(std::move(response.object));
    }
    auto it = handlers_.find(response.request_id);
    if (it != handlers_.end()) {
      it->second(std::move(response.object));
      handlers_.erase(it);
    }
  }

  void on_update(td_api::object_ptr<td_api::Object> update) {
    switch (update->get_id()) {
      case td_api::updateAuthorizationState::ID:
        on_update(std::move(td::move_tl_object_as<td_api::updateAuthorizationState>(update)));
        break;
      case td_api::updateMessageSendSucceeded::ID:
        on_update(std::move(td::move_tl_object_as<td_api::updateMessageSendSucceeded>(update)));
        break;
    }
  }

  void on_update(td_api::object_ptr<td_api::updateAuthorizationState> update) {
    switch (update->authorization_state_->get_id()) {
      case td_api::authorizationStateWaitTdlibParameters::ID:
        set_tdlib_parameters();
        break;
      case td_api::authorizationStateWaitPhoneNumber::ID:
        set_bot_token();
        break;
      case td_api::authorizationStateReady::ID:
        is_authorized_ = true;
        break;
    }
  }

  void on_update(td_api::object_ptr<td_api::updateMessageSendSucceeded> update) {
    if (update->old_message_id_ == message_id_) {
      timestamps_[3] = std::time(0);
      upload_finished_ = true;
    }
  }

  void set_tdlib_parameters() {
    auto request = td_api::make_object<td_api::setTdlibParameters>();
    request->database_directory_ = "tdlib";
    request->use_message_database_ = true;
    request->use_secret_chats_ = true;
    request->api_id_ = atoi(std::getenv("API_ID"));
    request->api_hash_ = std::getenv("API_HASH");
    request->system_language_code_ = "en";
    request->device_model_ = "TDLib";
    request->application_version_ = "1.0";
    query_must_succeed(std::move(request));
  }

  void set_bot_token() {
    query_must_succeed(td_api::make_object<td_api::checkAuthenticationBotToken>(std::getenv("BOT_TOKEN")));
  }

  bool fetched_message_{false};
  void fetch_message() {
    fetched_message_ = true;
    send_query(td_api::make_object<td_api::getMessageLinkInfo>(std::getenv("MESSAGE_LINK")),
               std::bind(&Client::on_get_message_link_info, this, std::placeholders::_1));
  }

  void on_get_message_link_info(td_api::object_ptr<td_api::Object> object) {
    if (!assert_id(td_api::messageLinkInfo::ID, object.get())) {
      return;
    }

    auto message_link_info = td::move_tl_object_as<td_api::messageLinkInfo>(object);
    if (message_link_info->message_ == nullptr) {
      std::cout << "Message not found." << std::endl;
      must_quit_ = true;
      return;
    } else if (message_link_info->message_->content_->get_id() != td_api::messageDocument::ID) {
      std::cout << "Invalid message type." << std::endl;
      must_quit_ = true;
      return;
    }

    auto message_document = td::move_tl_object_as<td_api::messageDocument>(message_link_info->message_->content_);
    file_id_ = message_document->document_->document_->id_;
  }

  bool download_started_{false};
  void start_download() {
    download_started_ = true;
    timestamps_[0] = std::time(0);
    send_query(td_api::make_object<td_api::downloadFile>(file_id_, 32, 0, 0, true),
               std::bind(&Client::on_file_downloaded, this, std::placeholders::_1));
  }

  void on_file_downloaded(td_api::object_ptr<td_api::Object> object) {
    if (!assert_id(td_api::file::ID, object.get())) {
      return;
    }

    timestamps_[1] = std::time(0);
    file_ = std::move(td::move_tl_object_as<td_api::file>(object));

    if (std::rename(file_->local_->path_.c_str(), file_name_) < 0) {
      std::cout << std::strerror(errno) << std::endl;
      must_quit_ = true;
    }
  }

  bool upload_started_{false};
  void start_upload() {
    upload_started_ = true;

    auto input_message_document = td_api::make_object<td_api::inputMessageDocument>();
    input_message_document->document_ = td_api::make_object<td_api::inputFileLocal>(file_name_);

    auto request = td_api::make_object<td_api::sendMessage>();
    request->chat_id_ = strtoll(std::getenv("CHAT_ID"), nullptr, 10);
    request->input_message_content_ = std::move(input_message_document);

    send_query(std::move(request), std::bind(&Client::on_upload_started, this, std::placeholders::_1));
  }

  void on_upload_started(td_api::object_ptr<td_api::Object> object) {
    if (!assert_id(td_api::message::ID, object.get())) {
      return;
    }

    timestamps_[2] = std::time(0);
    auto message_ = td::move_tl_object_as<td_api::message>(object);
    message_id_ = message_->id_;
  }

  void write_results() {
    std::ofstream results("results.json");
    results << '[' << std::to_string(file_->size_) << ",[" << std::to_string(timestamps_[0]) << ','
            << std::to_string(timestamps_[1]) << ',' << std::to_string(timestamps_[2]) << ','
            << std::to_string(timestamps_[3]) << "]]";
    results.close();
  }
};

int main() {
  Client client;
  client.loop();
}
