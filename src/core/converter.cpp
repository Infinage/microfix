#include "mfix/converter.hpp"
#include <unordered_set>
#include <charconv>

namespace {
    template<typename T> [[nodiscard]] 
    std::optional<T> _convert(std::string_view value) {
        T result {};
        auto *beg = value.data(), *end = beg + value.size();
        auto [ptr, ec] = std::from_chars(beg, end, result);
        if (ec == std::errc{} && ptr == end) return result;
        return std::nullopt;
    }
}

namespace mfix {
    Time::Time(std::chrono::milliseconds total) {
        hr = std::chrono::duration_cast<std::chrono::hours>(total);
        total -= hr;
        mn = std::chrono::duration_cast<std::chrono::minutes>(total);
        total -= mn;
        sc = std::chrono::duration_cast<std::chrono::seconds>(total);
        total -= sc;
        ms = total;
    }

    std::chrono::milliseconds Time::count() const { 
        return hr + mn + sc + ms;
    }

    TZTime::TZTime(std::chrono::milliseconds total, std::chrono::minutes off):
        Time{total} { this->offset = off; };

    std::chrono::milliseconds TZTime::count() const {
        return Time::count() - offset;
    }

    TZTimestamp::TZTimestamp(
        std::chrono::year_month_day ymd, 
        std::chrono::milliseconds total, 
        std::chrono::minutes off
    ): date {ymd}, time{total, off} {};

    std::chrono::system_clock::time_point 
    TZTimestamp::to_sys_time() const {
        auto days = std::chrono::sys_days{date};
        return std::chrono::system_clock::time_point{days + time.count()};
    }
}

namespace mfix::convert {
    std::optional<int64_t> to_int(const Field &field) {
        return _convert<int64_t>(field.value); 
    }

    std::optional<uint64_t> to_uint(const Field &field) {
        return _convert<uint64_t>(field.value); 
    }

    std::optional<double> to_double(const Field &field) {
        return _convert<double>(field.value); 
    }

    std::optional<char> to_char(const Field &field) {
        if (field.value.size() == 1) return field.value[0];
        return std::nullopt;
    }

    std::optional<bool> to_bool(const Field &field) {
        if (field.value == "Y") return true;
        else if (field.value == "N") return false;
        return std::nullopt;
     }

    std::optional<std::vector<char>> to_char_vector(const Field &field) {
        if (field.value.empty() || field.value.size() % 2 == 0) 
            return std::nullopt;

        auto expectedCount = (field.value.size() + 1) / 2;
        std::unordered_set<char> set;
        set.reserve(expectedCount);
        std::vector<char> result;
        result.reserve(expectedCount);

        for (std::size_t i {}; i < field.value.size(); ++i) {
            const char ch = field.value.at(i);
            if (i % 2 && ch != ' ') 
                return std::nullopt;
            else if (i % 2 == 0 && set.contains(ch)) 
                return std::nullopt;
            else if (i % 2 == 0) {
                set.insert(ch);
                result.push_back(ch);
            }
        }

        return result;
    }

    std::optional<std::vector<std::string>> to_str_vector(const Field &field) {
        if (field.value.empty()) return std::nullopt;

        std::unordered_set<std::string> set;
        std::vector<std::string> result;
        std::string acc;

        bool prevSpace = true;
        for (auto ch: field.value) {
            if (ch != ' ') {
                acc += ch, prevSpace = false;
            } else if (prevSpace || set.contains(acc)) {
                return std::nullopt;
            } else {
                result.push_back(acc);
                set.insert(std::move(acc));
                prevSpace = true;
                acc.clear();
            }
        }

        if (prevSpace || set.contains(acc)) 
            return std::nullopt;

        result.push_back(std::move(acc));
        return result;
    }

    std::optional<Date> to_date(const Field &field) {
        std::chrono::year_month_day ymd {};
        std::istringstream iss {field.value};
        iss >> std::chrono::parse("%Y%m%d", ymd);
        if (iss.fail() || iss.peek() != EOF) 
            return std::nullopt;
        return Date{ymd};
    }

    std::optional<Time> to_time(const Field &field) {
        std::string_view val = field.value;
        if (val.size() != 8 || val.at(2) != ':' || val.at(5) != ':')
             return std::nullopt;

        auto hr = _convert<unsigned>(val.substr(0, 2));
        auto mn = _convert<unsigned>(val.substr(3, 2));
        auto sc = _convert<unsigned>(val.substr(6, 2));

        if (!hr || !mn || !sc || *hr >= 24 || *mn >= 60 || *sc >= 60)
            return std::nullopt;

        auto total_sc = std::chrono::seconds{*hr * 60 * 60 + *mn * 60 + *sc};
        return Time{total_sc};
    }

    std::optional<TZTime> to_tztime(const Field &field) {
        std::chrono::seconds total_sc {};
        std::chrono::minutes offset_mins {};

        std::string tmp = field.value;
        if (!tmp.empty() && tmp.back() == 'Z') {
            tmp.pop_back(); tmp += "+00";
        }

        std::istringstream iss {tmp};
        iss >> std::chrono::parse("%T%Ez", total_sc, offset_mins);
        if (iss.fail()) {
            iss.clear();
            iss.str(tmp);
            iss >> std::chrono::parse("%R%Ez", total_sc, offset_mins);
            if (iss.fail()) return std::nullopt;
        }

        auto abs_offset = std::abs(offset_mins.count());
        if (iss.peek() != EOF || abs_offset > 12 * 60) 
            return std::nullopt;

        return TZTime{total_sc, offset_mins};
    }

    std::optional<mfix::MonthYear> to_monthyear(const Field &field) {
        // Parse the mandatory section: YYYYMM
        std::istringstream iss {field.value};
        std::chrono::year_month ym {};
        iss >> std::chrono::parse("%Y%m", ym);
        if (iss.fail()) return std::nullopt;

        // Handle: YYYYMM
        mfix::MonthYear result{.month=ym.month(), .year=ym.year()};
        if (field.value.size() == 6) return result;

        // Handle: YYYYMMDD
        else if (field.value.size() == 8 && std::isdigit(field.value[6])) {
            unsigned d_val = 0;
            auto beg = field.value.data() + 6, end = beg + 2;
            auto [ptr, ec] = std::from_chars(beg, end, d_val);
            if (ec != std::errc{} || ptr != end) return std::nullopt;
            else {
                result.day = std::chrono::day{d_val};
                if (!result.day->ok()) return std::nullopt;
            }
        }

        // Handle: YYYYMMwN
        else if (field.value.size() == 8 && (field.value[6] == 'w')) {
            int w_val = field.value[7] - '0';
            if (w_val < 1 || w_val > 5) return std::nullopt;
            result.week = w_val;
        }

        else if (field.value.size() != 6) {
            return std::nullopt;
        }

        return result;
    }

    std::optional<TZTimestamp> to_tztimestamp(const Field &field) {
        std::chrono::year_month_day ymd {};
        std::chrono::milliseconds total_ms {};
        std::chrono::minutes offset_mins {};

        std::string tmp = field.value;
        if (!tmp.empty() && tmp.back() == 'Z') {
            tmp.pop_back(); tmp += "+00";
        }

        char seperator {};
        std::istringstream iss {tmp};
        iss >> std::chrono::parse("%Y%m%d", ymd) >> seperator
            >> std::chrono::parse("%T%Ez", total_ms, offset_mins);

        if (iss.fail() || seperator != '-' || iss.peek() != EOF) 
            return std::nullopt;

        return TZTimestamp{ymd, total_ms, offset_mins};
    }
}
