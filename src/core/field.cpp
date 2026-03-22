#include "mfix/field.hpp"
#include "unordered_set"

namespace mfix {
    std::chrono::system_clock::time_point 
    TZTimestamp::to_sys_time() const {
        auto days = std::chrono::sys_days{date};
        return std::chrono::system_clock::time_point{days + time.count()};
    }

    template<>
    std::optional<std::string> Field::get() const {
        return value;
    }

    template<> 
    std::optional<char> Field::get() const {
        if (value.size() == 1) return value[0];
        return std::nullopt;
    }

    template<> 
    std::optional<bool> Field::get() const {
        if (value == "Y") return true;
        else if (value == "N") return false;
        return std::nullopt;
    }

    template<> 
    std::optional<std::vector<char>> Field::get() const {
        if (value.empty()) return std::nullopt;

        auto expectedCount = (value.size() + 1) / 2;
        std::unordered_set<char> set;
        set.reserve(expectedCount);
        std::vector<char> result;
        result.reserve(expectedCount);

        for (std::size_t i {}; i < value.size(); ++i) {
            const char ch = value.at(i);
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

    template<>
    std::optional<std::vector<std::string>> Field::get() const {
        if (value.empty()) return std::nullopt;

        std::unordered_set<std::string> set;
        std::vector<std::string> result;
        std::string acc;

        bool prevSpace = true;
        for (auto ch: value) {
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

    template<> 
    std::optional<Date> Field::get() const {
        std::chrono::year_month_day ymd {};
        std::istringstream iss {value};
        iss >> std::chrono::parse("%Y%m%d", ymd);
        if (char leftOver; iss.fail() || iss >> leftOver) 
            return std::nullopt;
        return Date{ymd};
    }

    template<>
    std::optional<Time> Field::get() const {
        std::chrono::seconds total_sc {};
        std::istringstream iss {value};
        iss >> std::chrono::parse("%T", total_sc);
        if (char leftOver; iss.fail() || iss.get(leftOver))
            return std::nullopt;
        return Time{total_sc};
    }

    template<>
    std::optional<TZTime> Field::get() const {
        std::chrono::seconds total_sc {};
        std::chrono::minutes offset_mins {};

        std::string tmp = value;
        if (!tmp.empty() && tmp.back() == 'Z') {
            tmp.pop_back(); tmp += "+00";
        }

        std::istringstream iss {tmp};
        iss >> std::chrono::parse("%T%Ez", total_sc, offset_mins);
        if (iss.fail()) {
            iss.clear();
            iss.str(tmp);
            iss >> std::chrono::parse("%H:%M%Ez", total_sc, offset_mins);
            if (iss.fail()) return std::nullopt;
        }

        if (char leftOver; iss.get(leftOver)) 
            return std::nullopt;

        return TZTime{total_sc, offset_mins};
    }

    template<> 
    std::optional<TZTimestamp> Field::get() const {
        std::chrono::year_month_day ymd {};
        std::chrono::milliseconds total_ms {};
        std::chrono::minutes offset_mins {};

        std::string tmp = value;
        if (!tmp.empty() && tmp.back() == 'Z') {
            tmp.pop_back(); tmp += "+00";
        }

        char seperator {};
        std::istringstream iss {tmp};
        iss >> std::chrono::parse("%Y%m%d", ymd) >> seperator
            >> std::chrono::parse("%T%Ez", total_ms, offset_mins);

        if (char leftOver; iss.fail() || seperator != '-' || iss.get(leftOver)) 
            return std::nullopt;

        return TZTimestamp{ymd, total_ms, offset_mins};
    }

    template<>
    std::optional<mfix::MonthYear> Field::get() const {
        // Parse the mandatory section: YYYYMM
        std::istringstream iss {value};
        std::chrono::year_month ym {};
        iss >> std::chrono::parse("%Y%m", ym);
        if (iss.fail()) return std::nullopt;

        // Handle: YYYYMM
        mfix::MonthYear result{.month=ym.month(), .year=ym.year()};
        if (value.size() == 6) return result;

        // Handle: YYYYMMDD
        else if (value.size() == 8 && std::isdigit(value[6])) {
            unsigned d_val = 0;
            auto beg = value.data() + 6, end = beg + 2;
            auto [ptr, ec] = std::from_chars(beg, end, d_val);
            if (ec != std::errc{} || ptr != end) return std::nullopt;
            else {
                result.day = std::chrono::day{d_val};
                if (!result.day->ok()) return std::nullopt;
            }
        }

        // Handle: YYYYMMwN
        else if (value.size() == 8 && (value[6] == 'w' || value[6] == 'W')) {
            int w_val = value[7] - '0';
            if (w_val < 1 || w_val > 5) return std::nullopt;
            result.week = std::chrono::weeks{w_val};
        }

        else if (value.size() != 6) {
            return std::nullopt;
        }

        return result;
    }
}
