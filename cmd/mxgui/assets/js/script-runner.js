import { CodeJar } from './codejar.min.js';

const highlight = editor => {
    let code = editor.textContent;
    
    // Escape HTML
    code = code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    
    // Combine all patterns into a single Regex.
    // Group 1: Comments | Group 2: Variables | Group 3: Jumps | Group 4: Keywords
    const lexer = /(#.*)|(\$[\w\.\[\]]+)|\b(if|elif|else|while|endwhile|break|continue|exit|endif)\b|\b(connect|listen|disconnect|reset|seq|send|wait|expect|loadmsg|set|unset|isset|incr|decr|print|sleep|assert|include|waitstatus)\b/g;
    
    // Process the matches in one clean pass
    code = code.replace(lexer, (match, pComment, pVar, pJump, pKeyword) => {
        if (pComment) return `<span class="text-gray-500 font-semibold">${pComment}</span>`;
        if (pVar)     return `<span class="text-yellow-400">${pVar}</span>`;
        if (pJump)    return `<span class="text-purple-400 font-bold">${pJump}</span>`;
        if (pKeyword) return `<span class="text-blue-400 font-bold">${pKeyword}</span>`;
        return match;
    });
    
    editor.innerHTML = code;
};

export async function initCodeJar(editorDiv, hiddenInput) {
    try {
    const jar = CodeJar(editorDiv, highlight, { tab: '    ' });

    jar.onUpdate(code => { 
        hiddenInput.value = code; 
        localStorage.setItem("mxshell-script", code);
    });
    
    // Load initial state from storage or default
    const saved = localStorage.getItem("mxshell-script") || hiddenInput.value;
    jar.updateCode(saved);
    
    } catch (error) {
        console.error("CodeJar Init Error:", error);
        editorDiv.innerHTML = `<span class="text-red-500">Editor failed to load.</span>`;
    }
}
